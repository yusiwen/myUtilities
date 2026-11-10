package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sampleStatusJSON = `{
  "schema_version": "1.0",
  "generated_at": "2026-10-20T09:51:12Z",
  "poll_after_seconds": 30,
  "max_stale_seconds": 120,
  "page": {"name": "DeepSeek", "url": "https://status.deepseek.com"},
  "overall": {"status": "operational"},
  "ongoing_incidents": [],
  "in_progress_maintenances": [],
  "scheduled_maintenances": []
}`

func TestVerdictLevels(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		overall string
		level   string
		ok      bool
	}{
		{OverallOperational, LevelHealthy, true},
		{" Operational ", LevelHealthy, true},
		{OverallDegraded, LevelDegraded, false},
		{OverallPartialOutage, LevelOutage, false},
		{OverallFullOutage, LevelOutage, false},
		{OverallMaintenance, LevelMaintenance, false},
		{"unknown_value", LevelUnknown, false},
		{"", LevelUnknown, false},
	}

	for _, tc := range tests {
		t.Run(tc.overall, func(t *testing.T) {
			h := Verdict(&StatusSummary{Overall: OverallStatus{Status: tc.overall}}, now)
			if h.Level != tc.level {
				t.Fatalf("level = %q, want %q", h.Level, tc.level)
			}
			if h.OK != tc.ok {
				t.Fatalf("ok = %v, want %v", h.OK, tc.ok)
			}
			if h.Headline == "" || h.Detail == "" {
				t.Fatalf("headline/detail must be non-empty, got %q / %q", h.Headline, h.Detail)
			}
		})
	}
}

func TestVerdictNilSummary(t *testing.T) {
	h := Verdict(nil, time.Now())
	if h.Level != LevelUnknown || h.OK {
		t.Fatalf("nil summary: level=%q ok=%v", h.Level, h.OK)
	}
	if h.DataAgeSeconds != -1 {
		t.Fatalf("data age = %d, want -1 for unknown", h.DataAgeSeconds)
	}
}

func TestVerdictFreshness(t *testing.T) {
	base := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	summary := &StatusSummary{
		Overall:         OverallStatus{Status: OverallOperational},
		MaxStaleSeconds: 120,
		GeneratedAt:     StatusTime{Time: base.Add(-60 * time.Second)},
	}

	h := Verdict(summary, base)
	if h.Stale {
		t.Fatalf("60s old snapshot flagged stale with a 120s window")
	}
	if h.DataAgeSeconds != 60 {
		t.Fatalf("data age = %d, want 60", h.DataAgeSeconds)
	}

	h = Verdict(summary, base.Add(5*time.Minute))
	if !h.Stale {
		t.Fatalf("snapshot older than max_stale_seconds not flagged stale")
	}

	// A timestamp ahead of the local clock is clock skew, not staleness.
	future := &StatusSummary{
		Overall:     OverallStatus{Status: OverallOperational},
		GeneratedAt: StatusTime{Time: base.Add(time.Hour)},
	}
	h = Verdict(future, base)
	if h.Stale || h.DataAgeSeconds != 0 {
		t.Fatalf("future timestamp: stale=%v age=%d, want false/0", h.Stale, h.DataAgeSeconds)
	}
}

func TestVerdictCountsIncidents(t *testing.T) {
	summary := &StatusSummary{
		Overall: OverallStatus{Status: OverallDegraded},
		OngoingIncidents: []StatusIncident{
			{ID: "inc-1", Phase: "monitoring", Impact: "major"},
			{ID: "inc-2", Phase: "investigating", Impact: "minor"},
		},
		ScheduledMaintenances: []StatusMaintenance{{ID: "mnt-1"}},
	}
	h := Verdict(summary, time.Now())
	if h.IncidentCount != 2 || h.ScheduledCount != 1 {
		t.Fatalf("counts = incidents:%d scheduled:%d", h.IncidentCount, h.ScheduledCount)
	}
	if !strings.Contains(h.Detail, "2 ongoing incidents") {
		t.Fatalf("detail = %q, want incident count", h.Detail)
	}
}

func TestStatusClientParsesAndCaches(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("X-Status-Validated-At", "2026-10-20T09:51:12Z")
		fmt.Fprint(w, sampleStatusJSON)
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	client := NewStatusClient(srv.URL)
	client.now = func() time.Time { return now }

	first, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("first status: %v", err)
	}
	if !first.OK || first.Health.Level != LevelHealthy {
		t.Fatalf("first: ok=%v level=%q", first.OK, first.Health.Level)
	}
	if first.Cached || first.NotModified {
		t.Fatalf("first: cached=%v notModified=%v", first.Cached, first.NotModified)
	}
	if first.ValidatedAt != "2026-10-20T09:51:12Z" {
		t.Fatalf("validated at = %q", first.ValidatedAt)
	}
	// The sample snapshot is weeks old relative to the fake clock.
	if !first.Health.Stale {
		t.Fatalf("expected the sample snapshot to be stale")
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}

	// Within poll_after_seconds the cached snapshot is served without a request.
	now = now.Add(10 * time.Second)
	second, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("second status: %v", err)
	}
	if !second.Cached || second.NotModified {
		t.Fatalf("second: cached=%v notModified=%v, want true/false", second.Cached, second.NotModified)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests after cached call = %d, want 1", got)
	}
}

func TestStatusClientRevalidatesWithETag(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, sampleStatusJSON)
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	client := NewStatusClient(srv.URL)
	client.now = func() time.Time { return now }

	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("first status: %v", err)
	}
	now = now.Add(31 * time.Second) // past poll_after_seconds
	revalidated, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("revalidated status: %v", err)
	}
	if !revalidated.NotModified || revalidated.Cached {
		t.Fatalf("cached=%v notModified=%v, want false/true", revalidated.Cached, revalidated.NotModified)
	}
	if revalidated.Health.OverallStatus != OverallOperational {
		t.Fatalf("overall = %q", revalidated.Health.OverallStatus)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestStatusClientErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantSub string
	}{
		{
			name: "unexpected status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantSub: "unexpected status 500",
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"overall":`)
			},
			wantSub: "failed to parse",
		},
		{
			name: "missing overall status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"schema_version":"1.0"}`)
			},
			wantSub: "missing overall.status",
		},
		{
			name: "body too large",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"overall":{"status":"operational"},"padding":"`))
				w.Write([]byte(strings.Repeat("x", maxStatusBodyBytes)))
				w.Write([]byte(`"}`))
			},
			wantSub: "exceeds",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			_, err := NewStatusClient(srv.URL).Status(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestStatusClient304WithoutCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	_, err := NewStatusClient(srv.URL).Status(context.Background())
	if err == nil || !strings.Contains(err.Error(), "304") {
		t.Fatalf("error = %v, want 304 diagnosis", err)
	}
}

func TestStatusClientReusesCacheOnClockMove(t *testing.T) {
	// Once poll_after_seconds has elapsed the client revalidates; ensure the
	// ETag from the first response is actually sent.
	var sentIfNoneMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			sentIfNoneMatch = r.Header.Get("If-None-Match")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-42"`)
		fmt.Fprint(w, sampleStatusJSON)
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	client := NewStatusClient(srv.URL)
	client.now = func() time.Time { return now }

	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("first status: %v", err)
	}
	now = now.Add(time.Hour)
	if _, err := client.Status(context.Background()); err != nil {
		t.Fatalf("second status: %v", err)
	}
	if sentIfNoneMatch != `"etag-42"` {
		t.Fatalf("If-None-Match = %q, want %q", sentIfNoneMatch, `"etag-42"`)
	}
}

func TestStatusTimeTolerance(t *testing.T) {
	var summary StatusSummary
	payload := `{
	  "generated_at": "2026-10-20T09:51:12Z",
	  "overall": {"status": "degraded"},
	  "ongoing_incidents": [{
	    "id": "inc-1",
	    "name": "Elevated error rate",
	    "phase": "investigating",
	    "impact": "major",
	    "started_at": null,
	    "last_update": {"message": "We are investigating."},
	    "affected_components": [
	      {"id": "api", "name": "API", "group_name": "Core", "status": "degraded"}
	    ]
	  }],
	  "scheduled_maintenances": [{
	    "id": "mnt-1",
	    "name": "Database upgrade",
	    "scheduled_start_at": 1790000000,
	    "scheduled_end_at": "2026-12-01T04:00:00Z"
	  }]
	}`
	if err := json.Unmarshal([]byte(payload), &summary); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if summary.GeneratedAt.IsZero() {
		t.Fatalf("generated_at not parsed")
	}
	inc := summary.OngoingIncidents[0]
	if inc.Name != "Elevated error rate" || inc.Phase != "investigating" || inc.Impact != "major" {
		t.Fatalf("incident = %+v", inc)
	}
	if inc.StartedAt.IsZero() != true {
		t.Fatalf("null started_at should stay zero")
	}
	if inc.LastUpdate.Message != "We are investigating." {
		t.Fatalf("last update = %q", inc.LastUpdate.Message)
	}
	if len(inc.AffectedComponents) != 1 || inc.AffectedComponents[0].GroupName != "Core" {
		t.Fatalf("components = %+v", inc.AffectedComponents)
	}
	mnt := summary.ScheduledMaintenances[0]
	start, end := mnt.Window()
	if start.IsZero() || end.IsZero() {
		t.Fatalf("maintenance window not parsed: start=%v end=%v", start, end)
	}
}

func TestHandleStatus(t *testing.T) {
	health := &Health{OK: true, Level: LevelHealthy, OverallStatus: OverallOperational, Headline: "All systems operational"}
	okFetch := func(context.Context) (*StatusResult, error) {
		now := time.Now()
		return &StatusResult{OK: true, Health: health, FetchedAt: now.UTC()}, nil
	}

	t.Run("ok", func(t *testing.T) {
		mux := http.NewServeMux()
		registerStatusHandler(mux, okFetch, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/budget/status", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var body StatusResult
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !body.OK || body.Health == nil || body.Health.Level != LevelHealthy {
			t.Fatalf("body = %+v", body)
		}
	})

	t.Run("fetch error", func(t *testing.T) {
		mux := http.NewServeMux()
		registerStatusHandler(mux, func(context.Context) (*StatusResult, error) {
			return nil, fmt.Errorf("boom")
		}, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/budget/status", nil))

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["ok"] != false || body["error"] != "boom" {
			t.Fatalf("body = %v", body)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		mux := http.NewServeMux()
		registerStatusHandler(mux, okFetch, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/budget/status", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", rec.Code)
		}
	})
}
