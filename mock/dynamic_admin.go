package mock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"embed"
)

//go:embed frontend/dist
var mockFrontendFS embed.FS

var mockMimeTypes = map[string]string{
	".js":    "application/javascript",
	".css":   "text/css",
	".html":  "text/html; charset=utf-8",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".woff2": "font/woff2",
}

func MockFrontendHandler() http.Handler {
	subFS, err := fs.Sub(mockFrontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to get mock frontend sub filesystem: %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." || p == "__admin" {
			p = "index.html"
		}
		data, err := fs.ReadFile(subFS, p)
		if err != nil {
			data, err = fs.ReadFile(subFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		if ct, ok := mockMimeTypes[path.Ext(p)]; ok {
			w.Header().Set("Content-Type", ct)
		}
		w.Write(data)
	})
}

type adminHandler struct {
	router     *DynamicRouter
	configPath string
	verbose    bool
}

func newAdminHandler(router *DynamicRouter, configPath string, verbose bool) *adminHandler {
	return &adminHandler{router: router, configPath: configPath, verbose: verbose}
}

// NewMockAdminHandler creates an http.Handler that serves the mock admin frontend
// and CRUD API, backed by the given config file.
func NewMockAdminHandler(configPath string) (http.Handler, error) {
	endpoints, _, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}
	router := NewDynamicRouter(endpoints, nil, false)
	admin := newAdminHandler(router, configPath, false)
	router.admin = admin
	return admin, nil
}

func (h *adminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/__admin")

	switch {
	case path == "" || path == "/":
		r.URL.Path = "/"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		MockFrontendHandler().ServeHTTP(w, r)
	case strings.HasPrefix(path, "/assets/"):
		r.URL.Path = path
		w.Header().Del("Content-Type")
		MockFrontendHandler().ServeHTTP(w, r)
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

func (h *adminHandler) listEndpoints(w http.ResponseWriter, r *http.Request) {
	eps := h.router.List()
	json.NewEncoder(w).Encode(eps)
}

func (h *adminHandler) listLogs(w http.ResponseWriter) {
	json.NewEncoder(w).Encode(h.router.Logs())
}

func (h *adminHandler) createEndpoint(w http.ResponseWriter, r *http.Request) {
	var ep ManagedEndpoint
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid JSON: %v"}`, err), http.StatusBadRequest)
		return
	}
	ep.ID = generateID()
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

func (h *adminHandler) updateEndpoint(w http.ResponseWriter, r *http.Request) {
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

func (h *adminHandler) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
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

func (h *adminHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	eps := h.router.List()

	type configOut struct {
		Port      int                `json:"port"`
		Endpoints []*ManagedEndpoint `json:"endpoints"`
	}

	// Try to read existing port from config file
	port := 8084
	if data, err := os.ReadFile(h.configPath); err == nil {
		var existing struct {
			Port int `json:"port"`
		}
		if json.Unmarshal(data, &existing) == nil && existing.Port != 0 {
			port = existing.Port
		}
	}

	out := configOut{Port: port, Endpoints: eps}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"marshal config failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(h.configPath, data, 0644); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"write config failed: %v"}`, err), http.StatusInternalServerError)
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

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
