package watch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Config struct {
	GitAuth *GitAuthConfig `json:"git_auth,omitempty"`
}

type GitAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to find home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "mu")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}
	return filepath.Join(dir, "watch.json"), nil
}

func resolvePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func loadConfig(configPath string) (*Config, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
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

func saveConfig(path string, cfg *Config) error {
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func resolveGitAuth() *http.BasicAuth {
	user := os.Getenv("GIT_AUTH_USER")
	pass := os.Getenv("GIT_AUTH_PASS")
	if user != "" && pass != "" {
		return &http.BasicAuth{Username: user, Password: pass}
	}

	cfg, err := loadConfig("")
	if err == nil && cfg.GitAuth != nil && cfg.GitAuth.Username != "" && cfg.GitAuth.Password != "" {
		return &http.BasicAuth{Username: cfg.GitAuth.Username, Password: cfg.GitAuth.Password}
	}

	return nil
}
