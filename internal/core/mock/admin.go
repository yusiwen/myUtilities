package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AdminHandler serves the mock admin frontend and CRUD API, backed by the given
// config file. The frontend handler is injected to keep the embedded SPA
// decoupled from core.
type AdminHandler struct {
	router     *DynamicRouter
	configPath string
	verbose    bool
	frontend   http.Handler
}

// NewAdminHandler creates an http.Handler that serves the admin frontend and CRUD API.
func NewAdminHandler(router *DynamicRouter, configPath string, verbose bool, frontend http.Handler) *AdminHandler {
	return &AdminHandler{router: router, configPath: configPath, verbose: verbose, frontend: frontend}
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/__admin")

	switch {
	case path == "" || path == "/":
		r.URL.Path = "/"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.frontend.ServeHTTP(w, r)
	case strings.HasPrefix(path, "/assets/"):
		r.URL.Path = path
		w.Header().Del("Content-Type")
		h.frontend.ServeHTTP(w, r)
	case path == "/api/endpoints" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		h.listEndpoints(w, r)
	case path == "/api/endpoints" && r.Method == http.MethodPost:
		w.Header().Set("Content-Type", "application/json")
		h.createEndpoint(w, r)
	case strings.HasPrefix(path, "/api/endpoints/") && r.Method == http.MethodPut:
		w.Header().Set("Content-Type", "application/json")
		h.updateEndpoint(w, r)
	case strings.HasPrefix(path, "/api/endpoints/") && r.Method == http.MethodDelete:
		w.Header().Set("Content-Type", "application/json")
		h.deleteEndpoint(w, r)
	case path == "/api/save" && r.Method == http.MethodPost:
		w.Header().Set("Content-Type", "application/json")
		h.saveConfig(w, r)
	case path == "/api/logs":
		w.Header().Set("Content-Type", "application/json")
		h.listLogs(w)
	default:
		h.router.ServeHTTP(w, r)
	}
}

func (h *AdminHandler) listEndpoints(w http.ResponseWriter, r *http.Request) {
	eps := h.router.List()
	json.NewEncoder(w).Encode(eps)
}

func (h *AdminHandler) listLogs(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(h.router.Logs())
}

func (h *AdminHandler) createEndpoint(w http.ResponseWriter, r *http.Request) {
	var ep ManagedEndpoint
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}
	ep.ID = GenerateID()
	ep.Method = strings.ToUpper(ep.Method)
	if ep.Status == 0 {
		ep.Status = http.StatusOK
	}
	h.router.Add(&ep)

	if h.verbose {
		fmt.Printf("---\n→ POST /__admin/api/endpoints\n← 201  (created %s %s)\n", ep.Method, ep.Path)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ep)
}

func (h *AdminHandler) updateEndpoint(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/__admin/api/endpoints/")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	var ep ManagedEndpoint
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}
	ep.Method = strings.ToUpper(ep.Method)
	if ep.Status == 0 {
		ep.Status = http.StatusOK
	}

	if !h.router.Update(id, &ep) {
		http.Error(w, `{"error":"endpoint not found"}`, http.StatusNotFound)
		return
	}

	if h.verbose {
		fmt.Printf("---\n→ PUT /__admin/api/endpoints/%s\n← 200  (updated %s %s)\n", id, ep.Method, ep.Path)
	}

	json.NewEncoder(w).Encode(ep)
}

func (h *AdminHandler) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/__admin/api/endpoints/")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	if !h.router.Delete(id) {
		http.Error(w, `{"error":"endpoint not found"}`, http.StatusNotFound)
		return
	}

	if h.verbose {
		fmt.Printf("---\n→ DELETE /__admin/api/endpoints/%s\n← 204\n", id)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	eps := h.router.List()

	if err := SaveConfig(h.configPath, eps); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	if h.verbose {
		fmt.Printf("---\n→ POST /__admin/api/save\n← 200  (saved %d endpoints to %s)\n", len(eps), h.configPath)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"file":      h.configPath,
		"endpoints": len(eps),
	})
}
