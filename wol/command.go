package wol

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	corenet "github.com/yusiwen/myUtilities/core/net"
	corestore "github.com/yusiwen/myUtilities/core/store"
)

func (o *ServeOptions) Run() error {
	// Open KV store
	store, err := corestore.OpenStore(o.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %v", err)
	}
	defer store.Close()
	log.Printf("Using KV store at %s", o.DBPath)

	// Start HTTP server
	mux := http.NewServeMux()

	// Frontend static files (must be registered before API to handle /)
	mux.Handle("/", frontendHandler())

	mux.HandleFunc("/api/wake/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "POST method required"}`, http.StatusMethodNotAllowed)
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
		entry, err := store.Get(hostname)
		if err != nil {
			http.Error(w, `{"error": "hostname not found"}`, http.StatusNotFound)
			return
		}
		// Send WOL magic packet
		err = corenet.SendWOL(entry.Mac, o.Interface)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "WOL failed: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status": "ok", "host": "%s", "mac": "%s"}`, hostname, entry.Mac)
		log.Printf("WOL packet sent to %s (%s) via %s", hostname, entry.Mac, o.Interface)
	})

	// Alias management endpoints
	mux.HandleFunc("/api/aliases", func(w http.ResponseWriter, r *http.Request) {
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
			if !corenet.ValidHostname(req.Name) {
				http.Error(w, `{"error": "invalid hostname format"}`, http.StatusBadRequest)
				return
			}
			if !corenet.ValidMAC(req.Mac) {
				http.Error(w, `{"error": "invalid MAC address format (expected aa:bb:cc:dd:ee:ff)"}`, http.StatusBadRequest)
				return
			}
			req.Mac = strings.ToLower(req.Mac)
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

	mux.HandleFunc("/api/aliases/{name}", func(w http.ResponseWriter, r *http.Request) {
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

	// Boot notification/query endpoint
	mux.HandleFunc("/api/boot/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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
			now := time.Now()
			if err := store.RecordBoot(hostname, now); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to record boot: %v"}`, err), http.StatusInternalServerError)
				return
			}
			if err := store.SetStatus(hostname, "boot"); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to set status: %v"}`, err), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"status": "ok", "host": "%s", "boot_time": "%s"}`, hostname, now.Format(time.RFC3339))
			log.Printf("Boot notification received from %s at %s", hostname, now.Format(time.RFC3339))
		case http.MethodGet:
			t, err := store.GetBootTime(hostname)
			if err != nil {
				http.Error(w, `{"boot_time": ""}`, http.StatusOK)
				return
			}
			fmt.Fprintf(w, `{"status": "ok", "host": "%s", "boot_time": "%s"}`, hostname, t.Format(time.RFC3339))
		default:
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Shutdown notification/query endpoint
	mux.HandleFunc("/api/shutdown/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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
			now := time.Now()
			if err := store.SetStatus(hostname, "shutdown"); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to set status: %v"}`, err), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"status": "ok", "host": "%s", "shutdown_time": "%s"}`, hostname, now.Format(time.RFC3339))
			log.Printf("Shutdown notification received from %s at %s", hostname, now.Format(time.RFC3339))
		case http.MethodGet:
			status, err := store.GetStatus(hostname)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to get status: %v"}`, err), http.StatusInternalServerError)
				return
			}
			if status == "" {
				status = "unknown"
			}
			fmt.Fprintf(w, `{"status": "ok", "host": "%s", "state": "%s"}`, hostname, status)
		default:
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	addr := fmt.Sprintf(":%d", o.Port)
	log.Printf("Starting WOL HTTP server on %s, interface %s", addr, o.Interface)
	return http.ListenAndServe(addr, mux)
}

func (o *AgentOptions) Run() error {
	hostname := o.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %v", err)
		}
	}

	if o.Boot && o.Shutdown {
		return fmt.Errorf("agent: --boot and --shutdown are mutually exclusive")
	}

	// Default to boot notification if neither flag is specified
	action := "boot"
	apiPath := "boot"
	if o.Shutdown {
		action = "shutdown"
		apiPath = "shutdown"
	}

	server := o.Server
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}

	log.Printf("Agent: sending %s notification for hostname %q to server %s", action, hostname, o.Server)

	// Retry with backoff in case the server is not ready yet
	maxRetries := 5
	for i := range maxRetries {
		url := fmt.Sprintf("%s/api/%s/%s", server, apiPath, hostname)
		resp, err := http.Post(url, "application/json", nil)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("Agent: %s notification sent for %q at %s", action, hostname, o.Server)
				return nil
			}
			return fmt.Errorf("agent: server returned status %d for %s", resp.StatusCode, hostname)
		}
		if i < maxRetries-1 {
			wait := time.Duration(i+1) * time.Second
			log.Printf("Agent: attempt %d failed, retrying in %v: %v", i+1, wait, err)
			time.Sleep(wait)
		} else {
			return fmt.Errorf("agent: failed to send %s notification for %q after %d retries: %v", action, hostname, maxRetries, err)
		}
	}
	return nil
}

func (o *InterfacesOptions) Run() error {
	details, err := corenet.GetInterfaceDetails()
	if err != nil {
		return fmt.Errorf("failed to list interfaces: %v", err)
	}

	fmt.Printf("Available network interfaces (%d found):\n", len(details))
	fmt.Println(strings.Repeat("=", 60))

	for i, detail := range details {
		iface := detail.Interface
		fmt.Printf("%d. %s\n", i+1, iface.Name)

		if o.Verbose {
			// Show MAC address
			if iface.HardwareAddr != nil {
				fmt.Printf("   MAC: %s\n", iface.HardwareAddr)
			}

			// Show flags
			fmt.Printf("   Flags: %v\n", iface.Flags)

			// Show IP addresses
			if len(detail.Addrs) > 0 {
				fmt.Printf("   Addresses:\n")
				for _, addr := range detail.Addrs {
					fmt.Printf("     - %s\n", addr)
				}
			}

			// Show interface type
			if detail.Type != "" {
				fmt.Printf("   Type: %s\n", detail.Type)
			}

			// Show suitability for WOL
			if detail.Suitable {
				fmt.Printf("   ✓ Suitable for WOL\n")
			} else {
				fmt.Printf("   ✗ Not suitable for WOL\n")
			}

			fmt.Println()
		} else {
			// Brief info
			var info []string
			if iface.HardwareAddr != nil {
				info = append(info, fmt.Sprintf("MAC: %s", iface.HardwareAddr))
			}
			if detail.IPv4Count > 0 {
				info = append(info, fmt.Sprintf("IPv4: %d", detail.IPv4Count))
			}
			if len(info) > 0 {
				fmt.Printf("   (%s)\n", strings.Join(info, ", "))
			}
		}
	}

	// Show recommendation for WOL
	fmt.Println("\nRecommendation for WOL:")
	fmt.Println(strings.Repeat("-", 60))
	bestIface, err := corenet.SelectBestInterfaceForWOL()
	if err != nil {
		fmt.Printf("  Could not determine best interface: %v\n", err)
	} else {
		fmt.Printf("  Recommended interface: %s\n", bestIface.Name)
		if bestIface.HardwareAddr != nil {
			fmt.Printf("  MAC address: %s\n", bestIface.HardwareAddr)
		}

		// Show IP addresses
		addrs, _ := bestIface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				fmt.Printf("  IPv4 address: %s\n", ipNet.IP)
			}
		}

		fmt.Printf("\n  Use: mu wol serve %s\n", bestIface.Name)
	}

	return nil
}
