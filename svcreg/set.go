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
	Host   string `help:"Listen address."`
	Port   int    `help:"HTTP server port."`
	DBPath string `help:"BoltDB file path."`
	Path   string `name:"config" help:"Config file path. Default: ~/.config/mu/svcreg-config.json"`
}

func (o *ConfigSetOptions) Run() error {
	if o.Host == "" && o.Port == 0 && o.DBPath == "" {
		return fmt.Errorf("at least one of --host, --port, --db-path is required")
	}

	path := o.Path
	if path == "" {
		path = "~/.config/mu/svcreg-config.json"
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.Host != "" {
		cfg.Host = o.Host
	}
	if o.Port > 0 {
		cfg.Port = o.Port
	}
	if o.DBPath != "" {
		cfg.DBPath = o.DBPath
	}

	return SaveConfig(path, cfg)
}
