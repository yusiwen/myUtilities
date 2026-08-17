package metrics

import (
	"encoding/json"
	"net/http"
)

// RegisterManagedAPI registers the gateway-side endpoints that control a
// managed `mu metrics serve` subprocess. These routes live under the literal
// `/api/metrics/admin` prefix so they never collide with the read-only proxy
// wildcard `GET /api/metrics/{name}`.
func RegisterManagedAPI(mux *http.ServeMux, mgr *ManagedServer) {
	mux.HandleFunc("GET /api/metrics/admin/status", func(w http.ResponseWriter, r *http.Request) {
		writeManagedStatus(w, mgr)
	})
	mux.HandleFunc("POST /api/metrics/admin/start", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Start(); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeManagedStatus(w, mgr)
	})
	mux.HandleFunc("POST /api/metrics/admin/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Stop(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeManagedStatus(w, mgr)
	})
	mux.HandleFunc("POST /api/metrics/admin/restart", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Restart(); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeManagedStatus(w, mgr)
	})
}

func writeManagedStatus(w http.ResponseWriter, mgr *ManagedServer) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mgr.Status())
}
