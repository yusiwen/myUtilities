package es

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/elastic/go-elasticsearch/v8"
)

type serverState struct {
	mu         sync.RWMutex
	cfg        *ESConfig
	es         *elasticsearch.Client
	configPath string
}

func newServerState(configPath string) *serverState {
	return &serverState{configPath: configPath}
}

func (s *serverState) getClient() *elasticsearch.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.es
}

func (s *serverState) loadConfig() error {
	cfg, err := loadConfig(s.configPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	es, err := newESClient(cfg)
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

func (s *serverState) updateConfig(cfg *ESConfig) error {
	if err := saveConfig(s.configPath, cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	es, err := newESClient(cfg)
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

func (s *serverState) getConfig() *ESConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (o *SetHostOptions) Run() error {
	cfg, err := loadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Host = o.Host
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("ES host set to: %s\n", o.Host)
	return nil
}

func (o *SetUserOptions) Run() error {
	cfg, err := loadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Username = o.User
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("ES username set to: %s\n", o.User)
	return nil
}

func (o *SetPasswordOptions) Run() error {
	cfg, err := loadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Password = o.Password
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Println("ES password set")
	return nil
}

func (o *ServeOptions) Run() error {
	state := newServerState(o.Config)
	if err := state.loadConfig(); err != nil {
		log.Printf("Warning: could not load ES config: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", frontendHandler())

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
			cfg := state.getConfig()
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connected": false,
				"host":      cfg.Host,
				"error":     "not configured or connection failed",
			})
			return
		}
		info, err := esPing(es)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"connected": false,
				"host":      state.getConfig().Host,
				"error":     err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"connected": true,
			"host":      state.getConfig().Host,
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
		indices, err := esListIndices(es)
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
		result, err := esSearch(es, req.Index, req.Body)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg := state.getConfig()
			writeJSON(w, http.StatusOK, maskedPassword(cfg))
		case http.MethodPut:
			var req ESConfig
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
				return
			}
			cfg := state.getConfig()
			if req.Host != "" {
				cfg.Host = req.Host
			}
			if req.Username != "" {
				cfg.Username = req.Username
			}
			if req.Password != "" {
				cfg.Password = req.Password
			}
			if err := state.updateConfig(cfg); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, maskedPassword(cfg))
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	})

	host := o.Host
	if host == "" {
		host = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", host, o.Port)
	log.Printf("Starting ES search UI on http://%s", addr)
	return http.ListenAndServe(addr, mux)
}
