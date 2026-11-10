package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// statusFetcher retrieves one DeepSeek service status snapshot.
type statusFetcher func(context.Context) (*StatusResult, error)

// RegisterHandlers registers the budget API routes on the given mux. An empty
// statusURL uses the official DeepSeek status endpoint.
func RegisterHandlers(mux *http.ServeMux, configPath, statusURL string, debugf func(string, ...interface{})) {
	if debugf == nil {
		debugf = func(string, ...interface{}) {}
	}
	debugf("RegisterHandlers: registering GET /api/budget/balance, /api/budget/status, config=%s", configPath)
	mux.HandleFunc("/api/budget/balance", func(w http.ResponseWriter, r *http.Request) {
		handleBalance(w, r, configPath, debugf)
	})

	fetch := FetchDeepSeekStatus
	if statusURL != "" {
		fetch = NewStatusClient(statusURL).Status
	}
	registerStatusHandler(mux, fetch, debugf)
}

// registerStatusHandler mounts the service status endpoint. The fetcher is
// injected so tests can exercise the handler without network access.
func registerStatusHandler(mux *http.ServeMux, fetch statusFetcher, debugf func(string, ...interface{})) {
	if debugf == nil {
		debugf = func(string, ...interface{}) {}
	}
	mux.HandleFunc("/api/budget/status", func(w http.ResponseWriter, r *http.Request) {
		handleStatus(w, r, fetch, debugf)
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request, fetch statusFetcher, debugf func(string, ...interface{})) {
	// The browser polls this route; keep the JSON out of the browser cache and
	// let the server-side client decide when to revalidate upstream.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "GET required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := fetch(ctx)
	if err != nil {
		debugf("handleStatus: fetch failed: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	debugf("handleStatus: level=%s overall=%s cached=%v", result.Health.Level, result.Health.OverallStatus, result.Cached)
	json.NewEncoder(w).Encode(result)
}

func handleBalance(w http.ResponseWriter, r *http.Request, configPath string, debugf func(string, ...interface{})) {
	debugf("handleBalance: called, method=%s", r.Method)
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
		return
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		debugf("handleBalance: loadConfig failed: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	debugf("handleBalance: config loaded, providers=%d, debug_log=%v", len(cfg.Providers), cfg.DebugLog)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	results := FetchAllBalances(ctx, "", cfg, debugf)
	elapsed := time.Since(start)

	debugf("handleBalance: fetchBalances done, %d results, elapsed=%v", len(results), elapsed)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
	debugf("handleBalance: response written, done")
}
