package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RegisterHandlers registers the budget API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, configPath string, debugf func(string, ...interface{})) {
	if debugf == nil {
		debugf = func(string, ...interface{}) {}
	}
	debugf("RegisterHandlers: registering GET /api/budget/balance, config=%s", configPath)
	mux.HandleFunc("/api/budget/balance", func(w http.ResponseWriter, r *http.Request) {
		handleBalance(w, r, configPath, debugf)
	})
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
