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

	coremetrics "github.com/yusiwen/myUtilities/internal/core/metrics"
)

var debugEnabled bool

func debugLog(format string, args ...interface{}) {
	if debugEnabled {
		log.Printf("metrics: "+format, args...)
	}
}

func resolveDebug(flagVal bool, cfg *coremetrics.Config) bool {
	if flagVal {
		return true
	}
	return cfg != nil && cfg.DebugLog
}

func (o *ServeOptions) Run() error {
	cfg, err := coremetrics.LoadConfigDir(o.ConfigDir)
	if err != nil {
		return err
	}

	debugEnabled = resolveDebug(o.Debug, cfg)

	retention := coremetrics.ResolveRetention(o.Retention, cfg.Retention)
	interval := coremetrics.ResolveInterval(o.Interval, cfg.Interval)
	hostname := coremetrics.ResolveHostname(o.Hostname, cfg.Hostname)

	debugLog("Serve starting: port=%d, retention=%s, agent=%v, interval=%s, hostname=%s",
		o.Port, retention, o.Agent, interval, hostname)

	dbPath, err := coremetrics.ResolveDBPath(o.ConfigDir, cfg.DataDir, o.DBPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	debugLog("DB path: %s", dbPath)

	tsdb, err := coremetrics.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open tsdb: %w", err)
	}
	defer tsdb.Close()

	debugLog("TSDB opened: %s", dbPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if o.Agent {
		ag := coremetrics.NewAgent(coremetrics.AgentConfig{
			TSDB:      tsdb,
			Interval:  interval,
			Hostname:  hostname,
			Retention: retention,
			Debug:     debugEnabled,
		})
		go ag.Run(ctx)

		log.Printf("Agent started, interval=%s, hostname=%s", interval, hostname)
	}

	if retention > 0 {
		if err := tsdb.Compact(retention); err != nil {
			log.Printf("Compact on startup: %v", err)
		}
		log.Printf("Retention set to %s, compaction enabled", retention)
	}

	apiHandler := coremetrics.NewServer(tsdb, hostname, retention, debugEnabled)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/", FrontendHandler())

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
	cfg, err := coremetrics.LoadConfig("")
	if err != nil {
		return err
	}

	debugEnabled = resolveDebug(o.Debug, cfg)

	retention := coremetrics.ResolveRetention(o.Retention, cfg.Retention)
	interval := coremetrics.ResolveInterval(o.Interval, cfg.Interval)
	hostname := coremetrics.ResolveHostname(o.Hostname, cfg.Hostname)
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

	dataDir, err := coremetrics.DefaultDataDir()
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

	ag := coremetrics.NewAgent(coremetrics.AgentConfig{
		TSDB:      tsdb,
		ServerURL: serverURL,
		Interval:  interval,
		Hostname:  hostname,
		Retention: retention,
		Debug:     debugEnabled,
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
		respBody, _ := io.ReadAll(resp.Body)
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
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return io.ReadAll(resp.Body)
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
