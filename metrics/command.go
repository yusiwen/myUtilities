package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/shirou/gopsutil/v4/cpu"

	coremetrics "github.com/yusiwen/myUtilities/core/metrics"
	"github.com/yusiwen/myUtilities/core/metrics/collector"
)

const maxPushRetries = 3

var debugEnabled bool

func debugLog(format string, args ...interface{}) {
	if debugEnabled {
		log.Printf("metrics: "+format, args...)
	}
}

func resolveDebug(flagVal bool, cfg *MetricsConfig) bool {
	if flagVal {
		return true
	}
	return cfg != nil && cfg.DebugLog
}

func (o *ServeOptions) Run() error {
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}

	debugEnabled = resolveDebug(o.Debug, cfg)

	retention := resolveRetention(o.Retention, cfg.Retention)
	interval := resolveInterval(o.Interval, cfg.Interval)
	hostname := resolveHostname(o.Hostname, cfg.Hostname)

	debugLog("Serve starting: port=%d, retention=%s, agent=%v, interval=%s, hostname=%s",
		o.Port, retention, o.Agent, interval, hostname)

	dataDir, err := defaultDataDir()
	if err != nil {
		return err
	}
	if cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	debugLog("Data dir: %s", dataDir)

	dbPath := filepath.Join(dataDir, "metrics.db")
	tsdb, err := coremetrics.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open tsdb: %w", err)
	}
	defer tsdb.Close()

	debugLog("TSDB opened: %s", dbPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if o.Agent {
		ag := newAgent(agentConfig{
			tsdb:      tsdb,
			interval:  interval,
			hostname:  hostname,
			retention: retention,
			debug:     debugEnabled,
		})
		go ag.Run(ctx)

		log.Printf("Agent started, interval=%s, hostname=%s", interval, hostname)
	}

	// Compact on startup
	if retention > 0 {
		if err := tsdb.Compact(retention); err != nil {
			log.Printf("Compact on startup: %v", err)
		}
		log.Printf("Retention set to %s, compaction enabled", retention)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", handleListMetrics(tsdb))
	mux.HandleFunc("GET /api/metrics/{name}", handleQueryMetric(tsdb))
	mux.HandleFunc("POST /api/metrics/write", handleWrite(tsdb, hostname))
	mux.HandleFunc("POST /api/metrics/compact", handleCompact(tsdb, retention))

	addr := fmt.Sprintf(":%d", o.Port)
	log.Printf("Metrics server listening on %s", addr)

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		cancel()
		srv.Close()
	}()

	return srv.ListenAndServe()
}

func (o *AgentOptions) Run() error {
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}

	debugEnabled = resolveDebug(o.Debug, cfg)

	retention := resolveRetention(o.Retention, cfg.Retention)
	interval := resolveInterval(o.Interval, cfg.Interval)
	hostname := resolveHostname(o.Hostname, cfg.Hostname)
	serverURL := o.Server
	if serverURL == "" && cfg.ServerURL != "" {
		serverURL = cfg.ServerURL
	}

	if serverURL != "" {
		serverURL = strings.TrimRight(serverURL, "/")
		log.Printf("Agent mode: remote server=%s", serverURL)
	} else {
		log.Printf("Agent mode: local storage")
	}

	debugLog("Agent starting: server=%s, interval=%s, hostname=%s, retention=%s",
		serverURL, interval, hostname, retention)

	dataDir, err := defaultDataDir()
	if err != nil {
		return err
	}
	if cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	var tsdb *coremetrics.DB
	if serverURL == "" {
		dbPath := filepath.Join(dataDir, "metrics.db")
		tsdb, err = coremetrics.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open tsdb: %w", err)
		}
		defer tsdb.Close()

		debugLog("Local TSDB opened: %s", dbPath)

		if retention > 0 {
			if err := tsdb.Compact(retention); err != nil {
				log.Printf("Compact on startup: %v", err)
			}
		}
	}

	ag := newAgent(agentConfig{
		tsdb:       tsdb,
		serverURL:  serverURL,
		interval:   interval,
		hostname:   hostname,
		retention:  retention,
		debug:      debugEnabled,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	return ag.Run(ctx)
}

func (o *CompactOptions) Run() error {
	server := strings.TrimRight(o.Server, "/")
	payload := map[string]string{"retention": o.Retention}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(server+"/api/metrics/compact", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to server %s: %w", server, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		Ok        bool   `json:"ok"`
		Retention string `json:"retention"`
		Duration  string `json:"duration"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	log.Printf("Compaction done (retention=%s, duration=%s)", result.Retention, result.Duration)
	return nil
}

func (o *QueryOptions) Run() error {
	server := strings.TrimRight(o.Server, "/")

	if o.List {
		body, err := httpGet(server + "/api/metrics")
		if err != nil {
			return err
		}
		var names []string
		if err := json.Unmarshal(body, &names); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
		if len(names) == 0 {
			fmt.Println("No metrics found.")
			return nil
		}
		fmt.Println("Available metrics:")
		for _, name := range names {
			fmt.Printf("  %s\n", name)
		}
		return nil
	}

	if o.Name == "" {
		return fmt.Errorf("metric name is required (use --list to see available metrics)")
	}

	var from, to time.Time
	var err error
	if o.From != "" {
		from, err = time.Parse(time.RFC3339, o.From)
		if err != nil {
			from, err = time.Parse(time.RFC3339Nano, o.From)
		}
		if err != nil {
			return fmt.Errorf("invalid --from: %s", o.From)
		}
	} else {
		d, durErr := time.ParseDuration(o.Last)
		if durErr != nil {
			return fmt.Errorf("invalid --last: %s", o.Last)
		}
		from = time.Now().Add(-d)
	}

	if o.To == "now" || o.To == "" {
		to = time.Now()
	} else {
		to, err = time.Parse(time.RFC3339, o.To)
		if err != nil {
			to, err = time.Parse(time.RFC3339Nano, o.To)
		}
		if err != nil {
			return fmt.Errorf("invalid --to: %s", o.To)
		}
	}

	url := fmt.Sprintf("%s/api/metrics/%s?from=%s&to=%s&limit=%d",
		server, o.Name,
		from.Format(time.RFC3339Nano),
		to.Format(time.RFC3339Nano),
		o.Limit)
	if o.Tags != "" {
		url += "&tags=" + o.Tags
	}

	body, err := httpGet(url)
	if err != nil {
		return err
	}

	var metricsList []coremetrics.Metric
	if err := json.Unmarshal(body, &metricsList); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if len(metricsList) == 0 {
		fmt.Println("No data points found.")
		return nil
	}

	switch o.Format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(metricsList)
	case "csv":
		fmt.Println("metric,tags,timestamp,value")
		for _, m := range metricsList {
			tagStr := encodeTags(m.Tags)
			for _, p := range m.Points {
				ts := time.Unix(0, p.Timestamp).Format(time.RFC3339Nano)
				fmt.Printf("%s,%s,%s,%f\n", m.Name, tagStr, ts, p.Value)
			}
		}
	default: // table
		for _, m := range metricsList {
			fmt.Printf("Metric: %s\n", m.Name)
			if len(m.Tags) > 0 {
				fmt.Printf("Tags:   ")
				first := true
				for k, v := range m.Tags {
					if !first {
						fmt.Printf(", ")
					}
					fmt.Printf("%s=%s", k, v)
					first = false
				}
				fmt.Println()
			}
			fmt.Println()
			fmt.Printf("  %-3s  %-30s  %s\n", "#", "Timestamp", "Value")
			for i, p := range m.Points {
				ts := time.Unix(0, p.Timestamp).Format("2006-01-02 15:04:05 -07:00")
				fmt.Printf("  %-3d  %-30s  %f\n", i+1, ts, p.Value)
			}
			fmt.Println()
		}
	}

	return nil
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("connect to server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := readAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return io.ReadAll(resp.Body)
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

func parseTagsStr(s string) map[string]string {
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

func encodeTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	var parts []string
	for k, v := range tags {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func handleListMetrics(tsdb *coremetrics.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		debugLog("ListMetrics request")

		names, err := tsdb.ListMetrics()
		if err != nil {
			debugLog("ListMetrics failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if names == nil {
			names = []string{}
		}
		debugLog("ListMetrics returned %d names", len(names))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	}
}

func handleWrite(tsdb *coremetrics.DB, defaultHostname string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var reqs []coremetrics.WriteRequest
		if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
			debugLog("Write: invalid JSON: %v", err)
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		debugLog("Write: received %d data points", len(reqs))

		now := time.Now()
		writeBefore := time.Now()
		for _, req := range reqs {
			if req.Metric == "" {
				debugLog("Write: skipped empty metric name")
				continue
			}
			if req.Tags == nil {
				req.Tags = make(map[string]string)
			}
			if req.Tags["host"] == "" && defaultHostname != "" {
				req.Tags["host"] = defaultHostname
			}
			ts := now
			if req.Timestamp > 0 {
				ts = time.Unix(0, req.Timestamp)
			}
			if err := tsdb.Write(req.Metric, req.Tags, ts, req.Value); err != nil {
				debugLog("Write failed for %s: %v", req.Metric, err)
				http.Error(w, fmt.Sprintf("write failed: %v", err), http.StatusInternalServerError)
				return
			}
		}
		debugLog("Write: wrote %d points in %s", len(reqs), time.Since(writeBefore))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "written": len(reqs)})
	}
}

func handleCompact(tsdb *coremetrics.DB, defaultRetention time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retention := defaultRetention

		var body struct {
			Retention string `json:"retention"`
		}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&body)
			if body.Retention != "" {
				retention = coremetrics.ParseRetention(body.Retention)
			}
		}

		if retention <= 0 {
			debugLog("Compact: invalid retention=%s", retention)
			http.Error(w, "retention must be > 0", http.StatusBadRequest)
			return
		}

		debugLog("Compact: retention=%s", retention)

		before := time.Now()
		if err := tsdb.Compact(retention); err != nil {
			debugLog("Compact failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		debugLog("Compact done in %s", time.Since(before))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":        true,
			"retention": retention.String(),
			"duration":  time.Since(before).String(),
		})
	}
}

func handleQueryMetric(tsdb *coremetrics.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		from := parseTimeParam(r, "from", time.Now().Add(-1*time.Hour))
		to := parseTimeParam(r, "to", time.Now())
		limit := parseIntParam(r, "limit", 0)
		tags := parseTagsParam(r, "tags")

		debugLog("Query: name=%s, from=%s, to=%s, tags=%v, limit=%d", name, from.Format(time.RFC3339), to.Format(time.RFC3339), tags, limit)

		metrics, err := tsdb.Query(name, tags, from, to, limit)
		if err != nil {
			debugLog("Query failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		totalPoints := 0
		for _, m := range metrics {
			totalPoints += len(m.Points)
		}
		debugLog("Query returned %d metrics with %d points", len(metrics), totalPoints)

		w.Header().Set("Content-Type", "application/json")
		if len(metrics) == 0 {
			w.Write([]byte("[]"))
			return
		}
		json.NewEncoder(w).Encode(metrics)
	}
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

type agentConfig struct {
	tsdb       *coremetrics.DB
	serverURL  string
	interval   time.Duration
	hostname   string
	retention  time.Duration
	debug      bool
}

type Agent struct {
	cfg agentConfig

	registry  metrics.Registry
	collector *collector.OSCollector
}

func newAgent(cfg agentConfig) *Agent {
	return &Agent{
		cfg:       cfg,
		registry:  metrics.NewRegistry(),
		collector: collector.NewOSCollector(),
	}
}

func (a *Agent) Run(ctx context.Context) error {
	if err := a.collector.Collect(a.registry); err != nil {
		log.Printf("First collect: %v", err)
	}

	debugLog("Agent collect loop started, interval=%s", a.cfg.interval)

	hostTags := map[string]string{"host": a.cfg.hostname}

	ticker := time.NewTicker(a.cfg.interval)
	defer ticker.Stop()

	for {
		a.collectAndFlush(ctx, hostTags)

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil
		}
	}
}

func (a *Agent) collectAndFlush(ctx context.Context, hostTags map[string]string) {
	ts := time.Now()
	collectBefore := time.Now()

	if err := a.collector.Collect(a.registry); err != nil {
		log.Printf("Collect error: %v", err)
		return
	}

	debugLog("Collect done in %s", time.Since(collectBefore))

	tags := copyStringMap(hostTags)

	var batch []coremetrics.Metric
	a.registry.Each(func(name string, i interface{}) {
		var value float64
		switch m := i.(type) {
		case metrics.GaugeFloat64:
			value = m.Value()
		case metrics.Gauge:
			value = float64(m.Value())
		default:
			return
		}

		batch = append(batch, coremetrics.Metric{
			Name:   name,
			Tags:   tags,
			Points: []coremetrics.DataPoint{{Timestamp: ts.UnixNano(), Value: value}},
		})
	})

	debugLog("Collected %d metrics", len(batch))

	if a.cfg.serverURL != "" {
		a.pushToServer(ctx, batch)
	} else {
		writeBefore := time.Now()
		if err := a.cfg.tsdb.WriteBatch(batch); err != nil {
			log.Printf("WriteBatch error: %v", err)
		}
		debugLog("Local write done in %s", time.Since(writeBefore))
	}
}

func (a *Agent) pushToServer(ctx context.Context, batch []coremetrics.Metric) {
	var reqs []coremetrics.WriteRequest
	for _, m := range batch {
		for _, p := range m.Points {
			reqs = append(reqs, coremetrics.WriteRequest{
				Metric:    m.Name,
				Tags:      m.Tags,
				Timestamp: p.Timestamp,
				Value:     p.Value,
			})
		}
	}

	data, err := json.Marshal(reqs)
	if err != nil {
		log.Printf("Marshal error: %v", err)
		return
	}

	url := a.cfg.serverURL + "/api/metrics/write"
	backoff := 1 * time.Second

	for attempt := 0; attempt < maxPushRetries; attempt++ {
		if attempt > 0 {
			debugLog("Push retry attempt %d/%d, waiting %s", attempt+1, maxPushRetries, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
		}

		pushBefore := time.Now()
		resp, err := http.Post(url, "application/json", bytes.NewReader(data))
		if err == nil {
			resp.Body.Close()
			debugLog("Push success (%d metrics, took %s)", len(batch), time.Since(pushBefore))
			return
		}

		log.Printf("Push to server failed (attempt %d/%d): %v", attempt+1, maxPushRetries, err)
	}

	log.Printf("Server unreachable after %d retries, caching %d metrics locally", maxPushRetries, len(batch))
	if a.cfg.tsdb != nil {
		cacheBefore := time.Now()
		if err := a.cfg.tsdb.WriteBatch(batch); err != nil {
			log.Printf("Local cache write error: %v", err)
		}
		debugLog("Local cache write done in %s", time.Since(cacheBefore))
	}
}

func copyStringMap(m map[string]string) map[string]string {
	r := make(map[string]string, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

func resolveRetention(flagVal, cfgVal string) time.Duration {
	if flagVal != "0" {
		if d := coremetrics.ParseRetention(flagVal); d > 0 {
			return d
		}
	}
	return coremetrics.ParseRetention(cfgVal)
}

func resolveInterval(flagVal, cfgVal string) time.Duration {
	v := flagVal
	if v == "" || v == "30s" {
		v = cfgVal
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

func resolveHostname(flagVal, cfgVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if cfgVal != "" {
		return cfgVal
	}
	host, _ := os.Hostname()
	return host
}

func init() {
	cpu.Percent(0, false)
}
