package es

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
)

type esSetter struct{}

func init() {
	config.Register(&esSetter{})
}

func (s *esSetter) Name() string {
	return "es"
}

func (s *esSetter) Set(args []string) error {
	opts := &ConfigSetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set es"), kong.Description("Update Elasticsearch config."))
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
	ConfigHost     string `help:"Elasticsearch server URL."`
	ConfigUsername string `help:"Elasticsearch username."`
	ConfigPassword string `help:"Elasticsearch password."`
	ConfigPath     string `name:"config" help:"Config file path. Default: ~/.config/mu/es-config.json"`
}

func (o *ConfigSetOptions) Run() error {
	if o.ConfigHost == "" && o.ConfigUsername == "" && o.ConfigPassword == "" {
		return fmt.Errorf("at least one of --config-host, --config-username, --config-password is required")
	}

	path := o.ConfigPath
	if path == "" {
		path = "~/.config/mu/es-config.json"
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.ConfigHost != "" {
		cfg.Host = o.ConfigHost
	}
	if o.ConfigUsername != "" {
		cfg.Username = o.ConfigUsername
	}
	if o.ConfigPassword != "" {
		cfg.Password = o.ConfigPassword
	}

	return saveConfig(path, cfg)
}
