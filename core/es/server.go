package es

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/elastic/go-elasticsearch/v8"
)

// ServerState holds the ES client and config, safe for concurrent use.
type ServerState struct {
	mu         sync.RWMutex
	cfg        *Config
	es         *elasticsearch.Client
	configPath string
}

func NewServerState(configPath string) *ServerState {
	return &ServerState{configPath: configPath}
}

func (s *ServerState) getClient() *elasticsearch.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.es
}

func (s *ServerState) LoadConfig() error {
	cfg, err := LoadConfig(s.configPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	es, err := NewClient(cfg)
	if err != nil {
		s.es = nil
		s.mu.Unlock()
		return err
	}
	s.es = es
	s.mu.Unlock()
	log.Printf("ES client configured for %s", cfg.Host)
	return nil
}

func (s *ServerState) UpdateConfig(cfg *Config) error {
	if err := SaveConfig(s.configPath, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	es, err := NewClient(cfg)
	if err != nil {
		s.es = nil
		s.mu.Unlock()
		return err
	}
	s.es = es
	s.mu.Unlock()
	log.Printf("ES config updated: host=%s", cfg.Host)
	return nil
}

func (s *ServerState) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// RegisterHandlers registers the ES API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, state *ServerState) {
	writeJSON := func(w http.ResponseWriter, status int, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET required"})
			return
		}
		es := state.getClient()
		if es == nil {
			cfg := state.GetConfig()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connected": false,
				"host":      cfg.Host,
				"error":     "not configured or connection failed",
			})
			return
		}
		info, err := Ping(es)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connected": false,
				"host":      state.GetConfig().Host,
				"error":     err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connected": true,
			"host":      state.GetConfig().Host,
			"info":      info,
		})
	})

	mux.HandleFunc("/api/indices", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET required"})
			return
		}
		es := state.getClient()
		if es == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ES not connected"})
			return
		}
		indices, err := ListIndices(es)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"indices": indices})
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		es := state.getClient()
		if es == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ES not connected"})
			return
		}
		var req struct {
			Index string                 `json:"index"`
			Body  map[string]interface{} `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if req.Index == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "index is required"})
			return
		}
		if req.Body == nil {
			req.Body = map[string]interface{}{"query": map[string]interface{}{"match_all": map[string]interface{}{}}}
		}
		result, err := Search(es, req.Index, req.Body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg := state.GetConfig()
			writeJSON(w, http.StatusOK, MaskedConfig(cfg))
		case http.MethodPut:
			var req Config
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
				return
			}
			cfg := state.GetConfig()
			if req.Host != "" {
				cfg.Host = req.Host
			}
			if req.Username != "" {
				cfg.Username = req.Username
			}
			if req.Password != "" {
				cfg.Password = req.Password
			}
			if err := state.UpdateConfig(cfg); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, MaskedConfig(cfg))
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})
}
