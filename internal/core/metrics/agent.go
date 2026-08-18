package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/yusiwen/myUtilities/internal/core/metrics/collector"
)

const maxPushRetries = 3

type AgentConfig struct {
	TSDB        *DB
	ServerURL   string
	Interval    time.Duration
	Hostname    string
	Retention   time.Duration
	Debug       bool
	AutoCompact bool
}

type Agent struct {
	cfg       AgentConfig
	registry  metrics.Registry
	collector *collector.OSCollector
}

func NewAgent(cfg AgentConfig) *Agent {
	return &Agent{
		cfg:       cfg,
		registry:  metrics.NewRegistry(),
		collector: collector.NewOSCollector(),
	}
}

func (a *Agent) debugLog(format string, args ...interface{}) {
	if a.cfg.Debug {
		log.Printf("metrics: "+format, args...)
	}
}

// Run collects metrics on an interval and flushes to the server or local TSDB.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.collector.Collect(a.registry); err != nil {
		log.Printf("First collect: %v", err)
	}

	a.debugLog("Agent collect loop started, interval=%s", a.cfg.Interval)

	hostTags := map[string]string{"host": a.cfg.Hostname}

	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	var compactTicker *time.Ticker
	var compactC <-chan time.Time
	if a.cfg.AutoCompact && a.cfg.TSDB != nil && a.cfg.Retention > 0 {
		compactTicker = time.NewTicker(DefaultCompactInterval)
		defer compactTicker.Stop()
		compactC = compactTicker.C
	}

	for {
		a.collectAndFlush(ctx, hostTags)

		select {
		case <-ticker.C:
		case <-compactC:
			if err := a.cfg.TSDB.Compact(a.cfg.Retention); err != nil {
				log.Printf("Auto compact error: %v", err)
			}
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

	a.debugLog("Collect done in %s", time.Since(collectBefore))

	tags := copyStringMap(hostTags)

	var batch []Metric
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

		batch = append(batch, Metric{
			Name:   name,
			Tags:   tags,
			Points: []DataPoint{{Timestamp: ts.UnixNano(), Value: value}},
		})
	})

	a.debugLog("Collected %d metrics", len(batch))

	if a.cfg.ServerURL != "" {
		a.pushToServer(ctx, batch)
	} else {
		writeBefore := time.Now()
		if err := a.cfg.TSDB.WriteBatch(batch); err != nil {
			log.Printf("WriteBatch error: %v", err)
		}
		a.debugLog("Local write done in %s", time.Since(writeBefore))
	}
}

func (a *Agent) pushToServer(ctx context.Context, batch []Metric) {
	var reqs []WriteRequest
	for _, m := range batch {
		for _, p := range m.Points {
			reqs = append(reqs, WriteRequest{
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

	url := a.cfg.ServerURL + "/api/metrics/write"
	backoff := 1 * time.Second

	for attempt := 0; attempt < maxPushRetries; attempt++ {
		if attempt > 0 {
			a.debugLog("Push retry attempt %d/%d, waiting %s", attempt+1, maxPushRetries, backoff)
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
			a.debugLog("Push success (%d metrics, took %s)", len(batch), time.Since(pushBefore))
			return
		}

		log.Printf("Push to server failed (attempt %d/%d): %v", attempt+1, maxPushRetries, err)
	}

	log.Printf("Server unreachable after %d retries, caching %d metrics locally", maxPushRetries, len(batch))
	if a.cfg.TSDB != nil {
		cacheBefore := time.Now()
		if err := a.cfg.TSDB.WriteBatch(batch); err != nil {
			log.Printf("Local cache write error: %v", err)
		}
		a.debugLog("Local cache write done in %s", time.Since(cacheBefore))
	}
}

func copyStringMap(m map[string]string) map[string]string {
	r := make(map[string]string, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}
