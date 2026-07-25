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
	ConfigGitUser     string `help:"Git auth username."`
	ConfigGitPassword string `help:"Git auth password."`
	ConfigPath        string `name:"config" help:"Config file path. Default: ~/.config/mu/watch.json"`
}

func (o *SetOptions) Run() error {
	if o.ConfigGitUser == "" && o.ConfigGitPassword == "" {
		return fmt.Errorf("at least one of --config-git-user, --config-git-password is required")
	}

	path := o.ConfigPath
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

	if o.ConfigGitUser != "" {
		cfg.GitAuth.Username = o.ConfigGitUser
	}
	if o.ConfigGitPassword != "" {
		cfg.GitAuth.Password = o.ConfigGitPassword
	}

	return saveConfig(path, cfg)
}
