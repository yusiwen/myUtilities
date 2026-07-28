package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func (o *ServeOptions) Run() error {
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}

	retention := resolveRetention(o.Retention, cfg.Retention)
	interval := resolveInterval(o.Interval, cfg.Interval)
	hostname := resolveHostname(o.Hostname, cfg.Hostname)

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

	dbPath := filepath.Join(dataDir, "metrics.db")
	tsdb, err := coremetrics.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open tsdb: %w", err)
	}
	defer tsdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if o.Agent {
		ag := newAgent(agentConfig{
			tsdb:      tsdb,
			interval:  interval,
			hostname:  hostname,
			retention: retention,
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
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}

	retention := resolveRetention(o.Retention, cfg.Retention)
	if retention <= 0 {
		return fmt.Errorf("retention must be > 0 (e.g. --retention 30d)")
	}

	dataDir, err := defaultDataDir()
	if err != nil {
		return err
	}
	if cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}

	dbPath := filepath.Join(dataDir, "metrics.db")
	tsdb, err := coremetrics.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open tsdb: %w", err)
	}
	defer tsdb.Close()

	before := time.Now()
	if err := tsdb.Compact(retention); err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	log.Printf("Compaction done in %s (retention=%s)", time.Since(before), retention)
	return nil
}

func (o *QueryOptions) Run() error {
	cfg, err := loadConfig("")
	if err != nil {
		return err
	}

	dataDir, err := defaultDataDir()
	if err != nil {
		return err
	}
	if cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}

	dbPath := filepath.Join(dataDir, "metrics.db")
	tsdb, err := coremetrics.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open tsdb: %w", err)
	}
	defer tsdb.Close()

	if o.List {
		names, err := tsdb.ListMetrics()
		if err != nil {
			return err
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

	tags := parseTagsStr(o.Tags)
	limit := o.Limit
	if limit <= 0 {
		limit = 100
	}

	metricsList, err := tsdb.Query(o.Name, tags, from, to, limit)
	if err != nil {
		return err
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
		names, err := tsdb.ListMetrics()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if names == nil {
			names = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	}
}

func handleWrite(tsdb *coremetrics.DB, defaultHostname string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var reqs []coremetrics.WriteRequest
		if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		now := time.Now()
		for _, req := range reqs {
			if req.Metric == "" {
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
				http.Error(w, fmt.Sprintf("write failed: %v", err), http.StatusInternalServerError)
				return
			}
		}

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
			http.Error(w, "retention must be > 0", http.StatusBadRequest)
			return
		}

		before := time.Now()
		if err := tsdb.Compact(retention); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

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

		metrics, err := tsdb.Query(name, tags, from, to, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

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

	if err := a.collector.Collect(a.registry); err != nil {
		log.Printf("Collect error: %v", err)
		return
	}

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

	if a.cfg.serverURL != "" {
		a.pushToServer(ctx, batch)
	} else {
		if err := a.cfg.tsdb.WriteBatch(batch); err != nil {
			log.Printf("WriteBatch error: %v", err)
		}
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
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
		}

		resp, err := http.Post(url, "application/json", bytes.NewReader(data))
		if err == nil {
			resp.Body.Close()
			return
		}

		log.Printf("Push to server failed (attempt %d/%d): %v", attempt+1, maxPushRetries, err)
	}

	log.Printf("Server unreachable after %d retries, caching locally", maxPushRetries)
	if a.cfg.tsdb != nil {
		if err := a.cfg.tsdb.WriteBatch(batch); err != nil {
			log.Printf("Local cache write error: %v", err)
		}
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
