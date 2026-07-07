package watch

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Config struct {
	GitAuth *GitAuthConfig `json:"git_auth,omitempty"`
}

type GitAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mu", "watch.json")
}

func loadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func resolveGitAuth() *http.BasicAuth {
	user := os.Getenv("GIT_AUTH_USER")
	pass := os.Getenv("GIT_AUTH_PASS")
	if user != "" && pass != "" {
		return &http.BasicAuth{Username: user, Password: pass}
	}

	cfg, err := loadConfig()
	if err == nil && cfg.GitAuth != nil && cfg.GitAuth.Username != "" && cfg.GitAuth.Password != "" {
		return &http.BasicAuth{Username: cfg.GitAuth.Username, Password: cfg.GitAuth.Password}
	}

	return nil
}
