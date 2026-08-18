package metrics

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Mode constants reported by the status UDS / info endpoint.
const (
	ModeServer          = "server"
	ModeServerWithAgent = "server-with-agent"
	ModeAgentLocal      = "agent-local"
	ModeAgentRemote     = "agent-remote"
)

// ServerInfo describes a running serve/agent process. It is served both by the
// HTTP /api/metrics/info endpoint and the per-process Unix socket.
type ServerInfo struct {
	Mode            string `json:"mode"`
	Pid             int    `json:"pid"`
	StartedAt       string `json:"started_at"`
	Version         string `json:"version"`
	ConfigDir       string `json:"config_dir"`
	ConfigFile      string `json:"config_file"`
	DBPath          string `json:"db_path"`
	Retention       string `json:"retention"`
	CompactInterval string `json:"compact_interval"`
	CollectInterval string `json:"collect_interval"`
	Hostname        string `json:"hostname"`
	Port            int    `json:"port,omitempty"`
	Server          string `json:"server,omitempty"`
	Agent           bool   `json:"agent"`
	Debug           bool   `json:"debug"`
	Series          int64  `json:"series,omitempty"`
	Points          int64  `json:"points,omitempty"`
}

// Server exposes the metrics TSDB over HTTP.
type Server struct {
	tsdb             *DB
	info             ServerInfo
	defaultRetention time.Duration
	debug            bool
}

// NewServer builds an http.Handler exposing the metrics API.
func NewServer(tsdb *DB, info ServerInfo) http.Handler {
	debug := info.Debug
	retention := ParseRetention(info.Retention)
	s := &Server{
		tsdb:             tsdb,
		info:             info,
		defaultRetention: retention,
		debug:            debug,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", s.handleListMetrics)
	mux.HandleFunc("GET /api/metrics/hosts", s.handleListHosts)
	mux.HandleFunc("GET /api/metrics/info", s.handleInfo)
	mux.HandleFunc("GET /api/metrics/{name}", s.handleQueryMetric)
	mux.HandleFunc("POST /api/metrics/write", s.handleWrite)
	mux.HandleFunc("POST /api/metrics/compact", s.handleCompact)
	return mux
}

// ServeStatusPayload serializes the status info for the Unix socket, filling
// in point/series counts from the TSDB.
func ServeStatusPayload(info ServerInfo, tsdb *DB) ([]byte, error) {
	if tsdb != nil {
		if stats, err := tsdb.Stats(); err == nil {
			info.Series = stats.Series
			info.Points = stats.Points
		}
	}
	return json.Marshal(info)
}

func (s *Server) debugLog(format string, args ...interface{}) {
	if s.debug {
		log.Printf("metrics: "+format, args...)
	}
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := s.info
	if stats, err := s.tsdb.Stats(); err == nil {
		info.Series = stats.Series
		info.Points = stats.Points
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) handleListMetrics(w http.ResponseWriter, r *http.Request) {
	s.debugLog("ListMetrics request")

	names, err := s.tsdb.ListMetrics()
	if err != nil {
		s.debugLog("ListMetrics failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if names == nil {
		names = []string{}
	}
	s.debugLog("ListMetrics returned %d names", len(names))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	s.debugLog("ListHosts request")

	hosts, err := s.tsdb.ListHosts()
	if err != nil {
		s.debugLog("ListHosts failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hosts == nil {
		hosts = []string{}
	}
	s.debugLog("ListHosts returned %d hosts", len(hosts))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hosts)
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	var reqs []WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		s.debugLog("Write: invalid JSON: %v", err)
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	s.debugLog("Write: received %d data points", len(reqs))

	now := time.Now()
	writeBefore := time.Now()
	for _, req := range reqs {
		if req.Metric == "" {
			s.debugLog("Write: skipped empty metric name")
			continue
		}
		if req.Tags == nil {
			req.Tags = make(map[string]string)
		}
		if req.Tags["host"] == "" && s.info.Hostname != "" {
			req.Tags["host"] = s.info.Hostname
		}
		ts := now
		if req.Timestamp > 0 {
			ts = time.Unix(0, req.Timestamp)
		}
		if err := s.tsdb.Write(req.Metric, req.Tags, ts, req.Value); err != nil {
			s.debugLog("Write failed for %s: %v", req.Metric, err)
			http.Error(w, fmt.Sprintf("write failed: %v", err), http.StatusInternalServerError)
			return
		}
	}
	s.debugLog("Write: wrote %d points in %s", len(reqs), time.Since(writeBefore))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "written": len(reqs)})
}

func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	retention := s.defaultRetention

	var body struct {
		Retention string `json:"retention"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
		if body.Retention != "" {
			retention = ParseRetention(body.Retention)
		}
	}

	if retention <= 0 {
		s.debugLog("Compact: invalid retention=%s", retention)
		http.Error(w, "retention must be > 0", http.StatusBadRequest)
		return
	}

	s.debugLog("Compact: retention=%s", retention)

	before := time.Now()
	if err := s.tsdb.Compact(retention); err != nil {
		s.debugLog("Compact failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.debugLog("Compact done in %s", time.Since(before))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"retention": retention.String(),
		"duration":  time.Since(before).String(),
	})
}

func (s *Server) handleQueryMetric(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	from := parseTimeParam(r, "from", time.Now().Add(-1*time.Hour))
	to := parseTimeParam(r, "to", time.Now())
	limit := parseIntParam(r, "limit", 0)
	tags := parseTagsParam(r, "tags")

	s.debugLog("Query: name=%s, from=%s, to=%s, tags=%v, limit=%d", name, from.Format(time.RFC3339), to.Format(time.RFC3339), tags, limit)

	metrics, err := s.tsdb.Query(name, tags, from, to, limit)
	if err != nil {
		s.debugLog("Query failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPoints := 0
	for _, m := range metrics {
		totalPoints += len(m.Points)
	}
	s.debugLog("Query returned %d metrics with %d points", len(metrics), totalPoints)

	w.Header().Set("Content-Type", "application/json")
	if len(metrics) == 0 {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(metrics)
}

func parseTimeParam(r *http.Request, name string, def time.Time) time.Time {
	s := r.URL.Query().Get(name)
	if s == "" {
		return def
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
	}
	if err != nil {
		return def
	}
	return t
}

func parseIntParam(r *http.Request, name string, def int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

func parseTagsParam(r *http.Request, name string) map[string]string {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil
	}
	return ParseTagsStr(s)
}

// ParseTagsStr parses a "k=v,k=v" string into a tag map.
func ParseTagsStr(s string) map[string]string {
	if s == "" {
		return nil
	}
	tags := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			tags[kv[0]] = kv[1]
		}
	}
	return tags
}
