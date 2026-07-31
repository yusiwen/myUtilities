package wol

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	corenet "github.com/yusiwen/myUtilities/core/net"
	corestore "github.com/yusiwen/myUtilities/core/store"
)

// Server holds the WOL HTTP server dependencies.
type Server struct {
	Store     *corestore.Store
	Interface string
	Token     string
}

// RegisterHandlers registers the WOL API routes on the given mux.
func (s *Server) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/wake/{hostname}", s.handleWake)
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/aliases", s.handleAliases)
	mux.HandleFunc("/api/aliases/{name}", s.handleAliasByID)
	mux.HandleFunc("/api/notify/{hostname}", s.handleNotify)
}

func (s *Server) requireToken(w http.ResponseWriter, r *http.Request) bool {
	if s.Token == "" {
		return true
	}
	if r.Header.Get("X-Auth-Token") == s.Token {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	return false
}

func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "POST method required"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.requireToken(w, r) {
		return
	}
	hostname := r.PathValue("hostname")
	if hostname == "" {
		http.Error(w, `{"error": "missing hostname"}`, http.StatusBadRequest)
		return
	}
	if !corenet.ValidHostname(hostname) {
		http.Error(w, `{"error": "invalid hostname format"}`, http.StatusBadRequest)
		return
	}
	entry, err := s.Store.Get(hostname)
	if err != nil {
		http.Error(w, `{"error": "hostname not found"}`, http.StatusNotFound)
		return
	}
	err = corenet.SendWOL(entry.Mac, s.Interface)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "WOL failed: %v"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "host": "%s", "mac": "%s"}`, hostname, entry.Mac)
	log.Printf("WOL packet sent to %s (%s) via %s", hostname, entry.Mac, s.Interface)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "POST method required"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.requireToken(w, r) {
		return
	}
	var req struct {
		Name string `json:"name"`
		Mac  string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Mac == "" {
		http.Error(w, `{"error": "name and mac are required"}`, http.StatusBadRequest)
		return
	}
	if !corenet.ValidHostname(req.Name) {
		http.Error(w, `{"error": "invalid hostname format"}`, http.StatusBadRequest)
		return
	}
	if !corenet.ValidMAC(req.Mac) {
		http.Error(w, `{"error": "invalid MAC address format (expected aa:bb:cc:dd:ee:ff)"}`, http.StatusBadRequest)
		return
	}
	req.Mac = strings.ToLower(req.Mac)
	if err := s.Store.Set(req.Name, req.Mac, ""); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "failed to register: %v"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"status": "ok", "name": "%s", "mac": "%s"}`, req.Name, req.Mac)
	log.Printf("Registered hostname %s with MAC %s", req.Name, req.Mac)
}

func (s *Server) handleAliases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		aliases, err := s.Store.List()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "failed to list aliases: %v"}`, err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(aliases)
	case http.MethodPost:
		if !s.requireToken(w, r) {
			return
		}
		var req struct {
			Name  string `json:"name"`
			Mac   string `json:"mac"`
			Iface string `json:"iface,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Mac == "" {
			http.Error(w, `{"error": "name and mac are required"}`, http.StatusBadRequest)
			return
		}
		if !corenet.ValidHostname(req.Name) {
			http.Error(w, `{"error": "invalid hostname format"}`, http.StatusBadRequest)
			return
		}
		if !corenet.ValidMAC(req.Mac) {
			http.Error(w, `{"error": "invalid MAC address format (expected aa:bb:cc:dd:ee:ff)"}`, http.StatusBadRequest)
			return
		}
		req.Mac = strings.ToLower(req.Mac)
		if err := s.Store.Set(req.Name, req.Mac, req.Iface); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "failed to set alias: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"status": "ok", "name": "%s", "mac": "%s"}`, req.Name, req.Mac)
	default:
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAliasByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error": "missing name"}`, http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if !s.requireToken(w, r) {
			return
		}
		if err := s.Store.Delete(name); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "failed to delete alias: %v"}`, err), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"status": "ok", "name": "%s"}`, name)
	default:
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNotify(w http.ResponseWriter, r *http.Request) {
	hostname := r.PathValue("hostname")
	if hostname == "" {
		http.Error(w, `{"error": "missing hostname"}`, http.StatusBadRequest)
		return
	}
	if !corenet.ValidHostname(hostname) {
		http.Error(w, `{"error": "invalid hostname format"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodPost:
		if !s.requireToken(w, r) {
			return
		}
		eventType := r.URL.Query().Get("type")
		if eventType != "boot" && eventType != "shutdown" {
			http.Error(w, `{"error": "type must be 'boot' or 'shutdown'"}`, http.StatusBadRequest)
			return
		}
		if reqMAC := r.URL.Query().Get("mac"); reqMAC != "" {
			if entry, err := s.Store.Get(hostname); err == nil && entry.Mac != reqMAC {
				log.Printf("WARN: MAC mismatch for %s: request=%s, stored=%s", hostname, reqMAC, entry.Mac)
			}
		}
		if err := s.Store.RecordEvent(hostname, eventType); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "failed to record event: %v"}`, err), http.StatusInternalServerError)
			return
		}
		log.Printf("%s notification received from %s", eventType, hostname)
		fmt.Fprintf(w, `{"status": "ok", "host": "%s", "event_type": "%s"}`, hostname, eventType)
	case http.MethodGet:
		events, _ := s.Store.GetEvents(hostname, 10)
		status := ""
		if len(events) > 0 {
			status = events[0].Type
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"host":   hostname,
			"status": status,
			"events": events,
		})
	default:
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
	}
}
