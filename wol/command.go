package wol

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/sabhiram/go-wol/wol"
)

func (o *Options) Run() error {
	// Open KV store
	store, err := OpenStore(o.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %v", err)
	}
	defer store.Close()
	log.Printf("Using KV store at %s", o.DBPath)

	// Start HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/wake/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "POST method required"}`, http.StatusMethodNotAllowed)
			return
		}
		hostname := r.PathValue("hostname")
		if hostname == "" {
			http.Error(w, `{"error": "missing hostname"}`, http.StatusBadRequest)
			return
		}
		entry, err := store.Get(hostname)
		if err != nil {
			http.Error(w, `{"error": "hostname not found"}`, http.StatusNotFound)
			return
		}
		// Send WOL magic packet
		err = sendWOL(entry.Mac, o.Interface)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "WOL failed: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status": "ok", "host": "%s", "mac": "%s"}`, hostname, entry.Mac)
		log.Printf("WOL packet sent to %s (%s) via %s", hostname, entry.Mac, o.Interface)
	})

	// Alias management endpoints
	mux.HandleFunc("/aliases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			aliases, err := store.List()
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to list aliases: %v"}`, err), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(aliases)
		case http.MethodPost:
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
			if err := store.Set(req.Name, req.Mac, req.Iface); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to set alias: %v"}`, err), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"status": "ok", "name": "%s", "mac": "%s"}`, req.Name, req.Mac)
		default:
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/aliases/{name}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, `{"error": "missing name"}`, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodDelete:
			if err := store.Delete(name); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to delete alias: %v"}`, err), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"status": "ok", "name": "%s"}`, name)
		default:
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	addr := fmt.Sprintf(":%d", o.Port)
	log.Printf("Starting WOL HTTP server on %s, interface %s", addr, o.Interface)
	return http.ListenAndServe(addr, mux)
}

// ipFromInterface returns a *net.UDPAddr from a network interface name.
func ipFromInterface(iface string) (*net.UDPAddr, error) {
	ief, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}

	addrs, err := ief.Addrs()
	if err == nil && len(addrs) <= 0 {
		err = fmt.Errorf("no address associated with interface %s", iface)
	}
	if err != nil {
		return nil, err
	}

	// Validate that one of the addrs is a valid network IP address.
	for _, addr := range addrs {
		switch ip := addr.(type) {
		case *net.IPNet:
			if ip.IP.To4() != nil {
				return &net.UDPAddr{
					IP: ip.IP,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no address associated with interface %s", iface)
}

func sendWOL(mac, iface string) error {
	// Default broadcast address and port
	bcastIP := "255.255.255.255"
	udpPort := "9"

	// Determine local address based on interface
	var localAddr *net.UDPAddr
	if iface != "" {
		var err error
		localAddr, err = ipFromInterface(iface)
		if err != nil {
			return err
		}
	}

	// Resolve broadcast address
	bcastAddr := fmt.Sprintf("%s:%s", bcastIP, udpPort)
	udpAddr, err := net.ResolveUDPAddr("udp", bcastAddr)
	if err != nil {
		return err
	}

	// Build magic packet
	mp, err := wol.New(mac)
	if err != nil {
		return err
	}

	// Marshal to bytes
	bs, err := mp.Marshal()
	if err != nil {
		return err
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", localAddr, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send packet
	n, err := conn.Write(bs)
	if err != nil {
		return err
	}
	if n != 102 {
		return fmt.Errorf("magic packet sent was %d bytes (expected 102 bytes sent)", n)
	}
	return nil
}
