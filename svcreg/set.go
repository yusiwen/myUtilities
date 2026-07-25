package svcreg

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
)

type svcregSetter struct{}

func init() {
	config.Register(&svcregSetter{})
}

func (s *svcregSetter) Name() string {
	return "svcreg"
}

func (s *svcregSetter) Set(args []string) error {
	opts := &ConfigSetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set svcreg"), kong.Description("Update service registry config."))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	if err != nil {
		return err
	}
	return opts.Run()
}

type ConfigSetOptions struct {
	ConfigHost   string `help:"Listen address."`
	ConfigPort   int    `help:"HTTP server port."`
	ConfigDBPath string `help:"BoltDB file path."`
	ConfigPath   string `name:"config" help:"Config file path. Default: ~/.config/mu/svcreg-config.json"`
}

func (o *ConfigSetOptions) Run() error {
	if o.ConfigHost == "" && o.ConfigPort == 0 && o.ConfigDBPath == "" {
		return fmt.Errorf("at least one of --config-host, --config-port, --config-db-path is required")
	}

	path := o.ConfigPath
	if path == "" {
		path = "~/.config/mu/svcreg-config.json"
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.ConfigHost != "" {
		cfg.Host = o.ConfigHost
	}
	if o.ConfigPort > 0 {
		cfg.Port = o.ConfigPort
	}
	if o.ConfigDBPath != "" {
		cfg.DBPath = o.ConfigDBPath
	}

	return SaveConfig(path, cfg)
}
