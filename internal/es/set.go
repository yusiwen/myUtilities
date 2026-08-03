package es

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/config"
	corees "github.com/yusiwen/myUtilities/internal/core/es"
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
	Host     string `help:"Elasticsearch server URL."`
	Username string `help:"Elasticsearch username."`
	Password string `help:"Elasticsearch password."`
	Path     string `name:"config" help:"Config file path. Default: ~/.config/mu/es-config.json"`
}

func (o *ConfigSetOptions) Run() error {
	if o.Host == "" && o.Username == "" && o.Password == "" {
		return fmt.Errorf("at least one of --host, --username, --password is required")
	}

	path := o.Path
	if path == "" {
		path = "~/.config/mu/es-config.json"
	}

	cfg, err := corees.LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.Host != "" {
		cfg.Host = o.Host
	}
	if o.Username != "" {
		cfg.Username = o.Username
	}
	if o.Password != "" {
		cfg.Password = o.Password
	}

	return corees.SaveConfig(path, cfg)
}
