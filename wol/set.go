package wol

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
)

type wolSetter struct{}

func init() {
	config.Register(&wolSetter{})
}

func (s *wolSetter) Name() string {
	return "wol"
}

func (s *wolSetter) Set(args []string) error {
	opts := &WolSetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set wol"), kong.Description("Update WOL config."))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	if err != nil {
		return err
	}
	return opts.Run()
}

type WolSetOptions struct {
	ConfigServer    string `help:"WOL HTTP server URL."`
	ConfigDBPath    string `help:"BoltDB file path."`
	ConfigPort      int    `help:"HTTP server port."`
	ConfigToken     string `help:"API auth token."`
	ConfigInterface string `help:"Network interface name."`
	ConfigHostname  string `help:"Hostname for agent registration."`
	ConfigPath      string `name:"config" help:"Config file path. Default: ~/.config/mu/wol-config.json"`
}

func (o *WolSetOptions) Run() error {
	if o.ConfigServer == "" && o.ConfigDBPath == "" && o.ConfigPort == 0 && o.ConfigToken == "" && o.ConfigInterface == "" && o.ConfigHostname == "" {
		return fmt.Errorf("at least one flag is required")
	}

	path := o.ConfigPath
	if path == "" {
		path = "~/.config/mu/wol-config.json"
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.ConfigServer != "" {
		cfg.Server = o.ConfigServer
	}
	if o.ConfigDBPath != "" {
		cfg.DBPath = o.ConfigDBPath
	}
	if o.ConfigPort > 0 {
		cfg.Port = o.ConfigPort
	}
	if o.ConfigToken != "" {
		cfg.Token = o.ConfigToken
	}
	if o.ConfigInterface != "" {
		cfg.Interface = o.ConfigInterface
	}
	if o.ConfigHostname != "" {
		cfg.Hostname = o.ConfigHostname
	}

	return saveConfig(path, cfg)
}
