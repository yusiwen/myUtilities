package wol

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	corenet "github.com/yusiwen/myUtilities/core/net"
	corestore "github.com/yusiwen/myUtilities/core/store"
	corewol "github.com/yusiwen/myUtilities/core/wol"
)

var setConfigPath string

func (o *ServeOptions) resolveConfig() {
	cfg, err := corewol.LoadConfig(o.Config)
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

func (o *ConfigOptions) AfterApply() error {
	setConfigPath = o.Config
	return nil
}

func (o *ConfigSetOptions) Run() error {
	cfg, err := corewol.LoadConfig(setConfigPath)
	if err != nil {
		return err
	}
	if err := corewol.SetConfigValue(cfg, o.Key, o.Value); err != nil {
		return err
	}
	if err := corewol.SaveConfig(setConfigPath, cfg); err != nil {
		return err
	}
	fmt.Printf("%s set to: %s\n", o.Key, o.Value)
	return nil
}

func (o *ConfigGetOptions) Run() error {
	cfg, err := corewol.LoadConfig(setConfigPath)
	if err != nil {
		return err
	}
	val, ok := corewol.GetConfigValue(cfg, o.Key)
	if !ok {
		return fmt.Errorf("unknown config key: %q (valid: server, interface, db-path, port, token, hostname)", o.Key)
	}
	fmt.Println(val)
	return nil
}

func (o *ConfigListOptions) Run() error {
	cfg, err := corewol.LoadConfig(setConfigPath)
	if err != nil {
		return err
	}
	fmt.Printf("Config: %s\n\n", setConfigPath)
	fmt.Printf("server    = %s\n", cfg.Server)
	fmt.Printf("interface = %s\n", cfg.Interface)
	fmt.Printf("db-path   = %s\n", cfg.DBPath)
	fmt.Printf("port      = %d\n", cfg.Port)
	if cfg.Token != "" {
		fmt.Printf("token     = %s\n", cfg.Token)
	}
	if cfg.Hostname != "" {
		fmt.Printf("hostname  = %s\n", cfg.Hostname)
	}
	return nil
}

// RegisterHandlers registers the WOL API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, store *corestore.Store, o *ServeOptions) {
	(&corewol.Server{Store: store, Interface: o.Interface, Token: o.Token}).RegisterHandlers(mux)
}

func (o *AgentOptions) resolveConfig() {
	cfg, err := corewol.LoadConfig(o.Config)
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

	return corewol.RunAgent(corewol.AgentConfig{
		Server:    o.Server,
		Token:     o.Token,
		Hostname:  o.Hostname,
		Interface: o.Interface,
		Boot:      o.Boot,
		Shutdown:  o.Shutdown,
		Register:  o.Register,
	})
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
			if iface.HardwareAddr != nil {
				fmt.Printf("   MAC: %s\n", iface.HardwareAddr)
			}
			fmt.Printf("   Flags: %v\n", iface.Flags)
			if len(detail.Addrs) > 0 {
				fmt.Printf("   Addresses:\n")
				for _, addr := range detail.Addrs {
					fmt.Printf("     - %s\n", addr)
				}
			}
			if detail.Type != "" {
				fmt.Printf("   Type: %s\n", detail.Type)
			}
			if detail.Suitable {
				fmt.Printf("   ✓ Suitable for WOL\n")
			} else {
				fmt.Printf("   ✗ Not suitable for WOL\n")
			}
			fmt.Println()
		} else {
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
