package wol

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	corenet "github.com/yusiwen/myUtilities/core/net"
	corestore "github.com/yusiwen/myUtilities/core/store"
)

func (o *ServeOptions) requireToken(w http.ResponseWriter, r *http.Request) bool {
	if o.Token == "" {
		return true
	}
	if r.Header.Get("X-Auth-Token") == o.Token {
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	return false
}

func (o *ServeOptions) resolveConfig() {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		log.Printf("Warning: could not load WOL config: %v", err)
		return
	}
	if o.Interface == "" {
		o.Interface = cfg.Interface
	}
	if o.DBPath == "" {
		o.DBPath = cfg.DBPath
	}
	if o.Port == 0 {
		o.Port = cfg.Port
	}
	if o.Token == "" {
		o.Token = cfg.Token
	}
}

func (o *ServeOptions) Run() error {
	o.resolveConfig()

	store, err := corestore.OpenStore(o.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %v", err)
	}
	defer store.Close()
	log.Printf("Using KV store at %s", o.DBPath)

	mux := http.NewServeMux()
	RegisterHandlers(mux, store, o)
	mux.Handle("/", FrontendHandler())

	addr := fmt.Sprintf(":%d", o.Port)
	log.Printf("Starting WOL HTTP server on %s, interface %s", addr, o.Interface)
	return http.ListenAndServe(addr, mux)
}

func (o *SetDBPathOptions) Run() error {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.DBPath = o.DBPath
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("WOL DB path set to: %s\n", o.DBPath)
	return nil
}

func (o *SetPortOptions) Run() error {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Port = o.Port
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("WOL port set to: %d\n", o.Port)
	return nil
}

func (o *SetTokenOptions) Run() error {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Token = o.Token
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Println("WOL token set")
	return nil
}

func (o *SetInterfaceOptions) Run() error {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Interface = o.Interface
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("WOL interface set to: %s\n", o.Interface)
	return nil
}

func (o *SetServerOptions) Run() error {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Server = o.Server
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("WOL server set to: %s\n", o.Server)
	return nil
}

func (o *SetHostnameOptions) Run() error {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Hostname = o.Hostname
	if err := saveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("WOL hostname set to: %s\n", o.Hostname)
	return nil
}

func RegisterHandlers(mux *http.ServeMux, store *corestore.Store, o *ServeOptions) {
	mux.HandleFunc("/api/wake/{hostname}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "POST method required"}`, http.StatusMethodNotAllowed)
			return
		}
		if !o.requireToken(w, r) {
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
		err = corenet.SendWOL(entry.Mac, o.Interface)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "WOL failed: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status": "ok", "host": "%s", "mac": "%s"}`, hostname, entry.Mac)
		log.Printf("WOL packet sent to %s (%s) via %s", hostname, entry.Mac, o.Interface)
	})

	mux.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "POST method required"}`, http.StatusMethodNotAllowed)
			return
		}
		if !o.requireToken(w, r) {
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
		if err := store.Set(req.Name, req.Mac, ""); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "failed to register: %v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"status": "ok", "name": "%s", "mac": "%s"}`, req.Name, req.Mac)
		log.Printf("Registered hostname %s with MAC %s", req.Name, req.Mac)
	})

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
			if !o.requireToken(w, r) {
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
			if !o.requireToken(w, r) {
				return
			}
			if err := store.Delete(name); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to delete alias: %v"}`, err), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"status": "ok", "name": "%s"}`, name)
		default:
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/notify/{hostname}", func(w http.ResponseWriter, r *http.Request) {
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
			if !o.requireToken(w, r) {
				return
			}
			eventType := r.URL.Query().Get("type")
			if eventType != "boot" && eventType != "shutdown" {
				http.Error(w, `{"error": "type must be 'boot' or 'shutdown'"}`, http.StatusBadRequest)
				return
			}
			if reqMAC := r.URL.Query().Get("mac"); reqMAC != "" {
				if entry, err := store.Get(hostname); err == nil && entry.Mac != reqMAC {
					log.Printf("WARN: MAC mismatch for %s: request=%s, stored=%s", hostname, reqMAC, entry.Mac)
				}
			}
			if err := store.RecordEvent(hostname, eventType); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "failed to record event: %v"}`, err), http.StatusInternalServerError)
				return
			}
			log.Printf("%s notification received from %s", eventType, hostname)
			fmt.Fprintf(w, `{"status": "ok", "host": "%s", "event_type": "%s"}`, hostname, eventType)
		case http.MethodGet:
			events, _ := store.GetEvents(hostname, 10)
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
	})
}

func postWithToken(url, token, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Auth-Token", token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return http.DefaultClient.Do(req)
}

func (o *AgentOptions) resolveConfig() {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		log.Printf("Warning: could not load WOL config: %v", err)
		return
	}
	if o.Server == "" {
		o.Server = cfg.Server
	}
	if o.Token == "" {
		o.Token = cfg.Token
	}
	if o.Hostname == "" {
		o.Hostname = cfg.Hostname
	}
	if o.Interface == "" {
		o.Interface = cfg.Interface
	}
}

func (o *AgentOptions) Run() error {
	o.resolveConfig()

	if o.Server == "" {
		return fmt.Errorf("agent: server URL is required either as an argument or in config file (~/.config/mu/wol-config.json)")
	}

	hostname := o.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %v", err)
		}
	}

	flags := 0
	if o.Boot {
		flags++
	}
	if o.Shutdown {
		flags++
	}
	if o.Register {
		flags++
	}
	if flags > 1 {
		return fmt.Errorf("agent: --boot, --shutdown, and --register are mutually exclusive")
	}

	server := o.Server
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}

	serverURL, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("agent: invalid server URL %q: %v", o.Server, err)
	}

	if o.Register {
		var macStr string
		if o.Interface != "" {
			iface, err := corenet.GetInterfaceByName(o.Interface)
			if err != nil {
				return fmt.Errorf("agent: %v", err)
			}
			if iface.HardwareAddr == nil {
				return fmt.Errorf("agent: interface %s has no MAC address", o.Interface)
			}
			macStr = iface.HardwareAddr.String()
		} else {
			mac, err := corenet.GetOutboundMAC(serverURL.Host)
			if err != nil {
				return fmt.Errorf("agent: unable to determine outbound MAC: %v", err)
			}
			macStr = mac.String()
		}

		log.Printf("Agent: registering hostname %q with MAC %s to server %s", hostname, macStr, o.Server)

		body := fmt.Sprintf(`{"name":"%s","mac":"%s"}`, hostname, macStr)
		maxRetries := 5
		for i := range maxRetries {
			resp, err := postWithToken(server+"/api/register", o.Token, "application/json", strings.NewReader(body))
			if err == nil {
				if resp.StatusCode == http.StatusCreated {
					resp.Body.Close()
					log.Printf("Agent: successfully registered %q (%s) at %s", hostname, macStr, o.Server)
					return nil
				}
				resp.Body.Close()
				return fmt.Errorf("agent: server returned status %d for registration of %s", resp.StatusCode, hostname)
			}
			if i < maxRetries-1 {
				wait := time.Duration(i+1) * time.Second
				log.Printf("Agent: attempt %d failed, retrying in %v: %v", i+1, wait, err)
				time.Sleep(wait)
			} else {
				return fmt.Errorf("agent: failed to register %q after %d retries: %v", hostname, maxRetries, err)
			}
		}
		return nil
	}

	// Boot or shutdown notification
	action := "boot"
	if o.Shutdown {
		action = "shutdown"
	}

	mac, err := corenet.GetOutboundMAC(serverURL.Host)
	if err != nil {
		log.Printf("Agent: warning: unable to determine outbound MAC: %v", err)
	}

	log.Printf("Agent: sending %s notification for hostname %q to server %s", action, hostname, o.Server)

	maxRetries := 5
	for i := range maxRetries {
		u := fmt.Sprintf("%s/api/notify/%s?type=%s", server, url.PathEscape(hostname), action)
		if len(mac) > 0 {
			u += "&mac=" + url.QueryEscape(mac.String())
		}
		resp, err := postWithToken(u, o.Token, "application/json", nil)
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

		fmt.Printf("\n  Use: mu wol serve --interface %s\n", bestIface.Name)
	}

	return nil
}
