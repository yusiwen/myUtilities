package wol

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
	corewol "github.com/yusiwen/myUtilities/core/wol"
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
	Server    string `help:"WOL HTTP server URL."`
	DBPath    string `help:"BoltDB file path."`
	Port      int    `help:"HTTP server port."`
	Token     string `help:"API auth token."`
	Interface string `help:"Network interface name."`
	Hostname  string `help:"Hostname for agent registration."`
	Path      string `name:"config" help:"Config file path. Default: ~/.config/mu/wol-config.json"`
}

func (o *WolSetOptions) Run() error {
	if o.Server == "" && o.DBPath == "" && o.Port == 0 && o.Token == "" && o.Interface == "" && o.Hostname == "" {
		return fmt.Errorf("at least one flag is required")
	}

	path := o.Path
	if path == "" {
		path = "~/.config/mu/wol-config.json"
	}

	cfg, err := corewol.LoadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.Server != "" {
		cfg.Server = o.Server
	}
	if o.DBPath != "" {
		cfg.DBPath = o.DBPath
	}
	if o.Port > 0 {
		cfg.Port = o.Port
	}
	if o.Token != "" {
		cfg.Token = o.Token
	}
	if o.Interface != "" {
		cfg.Interface = o.Interface
	}
	if o.Hostname != "" {
		cfg.Hostname = o.Hostname
	}

	return corewol.SaveConfig(path, cfg)
}
