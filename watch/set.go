package watch

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
)

type watchSetter struct{}

func init() {
	config.Register(&watchSetter{})
}

func (s *watchSetter) Name() string {
	return "watch"
}

func (s *watchSetter) Set(args []string) error {
	opts := &SetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set watch"), kong.Description("Update watch config."))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	if err != nil {
		return err
	}
	return opts.Run()
}

type SetOptions struct {
	GitUser     string `help:"Git auth username."`
	GitPassword string `help:"Git auth password."`
	Path        string `name:"config" help:"Config file path. Default: ~/.config/mu/watch.json"`
}

func (o *SetOptions) Run() error {
	if o.GitUser == "" && o.GitPassword == "" {
		return fmt.Errorf("at least one of --git-user, --git-password is required")
	}

	path := o.Path
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.GitAuth == nil {
		cfg.GitAuth = &GitAuthConfig{}
	}

	if o.GitUser != "" {
		cfg.GitAuth.Username = o.GitUser
	}
	if o.GitPassword != "" {
		cfg.GitAuth.Password = o.GitPassword
	}

	return saveConfig(path, cfg)
}
