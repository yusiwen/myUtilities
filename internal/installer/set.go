package installer

import (
	"fmt"

	"github.com/alecthomas/kong"
	coreconfig "github.com/yusiwen/myUtilities/internal/core/config"
	coreinst "github.com/yusiwen/myUtilities/internal/core/installer"
)

type installerSetter struct{}

func init() {
	coreconfig.Register(&installerSetter{})
}

func (s *installerSetter) Name() string {
	return "installer"
}

func (s *installerSetter) Set(args []string) error {
	opts := &ConfigSetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set installer"), kong.Description("Update installer config (installer-config.json)."))
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
	Token  string `help:"GitHub token used to query release assets."`
	Unset  bool   `help:"Remove the stored token."`
	Config string `name:"config" help:"Config file path. Default: ~/.config/mu/installer-config.json"`
}

func (o *ConfigSetOptions) Run() error {
	if o.Token == "" && !o.Unset {
		return fmt.Errorf("nothing to do: pass --token to set it, or --unset to remove it")
	}

	cfg, err := coreinst.LoadConfig(o.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.Unset {
		cfg.Token = ""
	}
	if o.Token != "" {
		cfg.Token = o.Token
	}

	return coreinst.SaveConfig(o.Config, cfg)
}
