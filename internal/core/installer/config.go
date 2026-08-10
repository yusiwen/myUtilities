package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds installer module settings persisted in installer-config.json.
type Config struct {
	Token string `json:"token,omitempty"`
}

const defaultConfigPath = "~/.config/mu/installer-config.json"

func ResolveConfigPath(raw string) (string, error) {
	path := raw
	if path == "" {
		path = defaultConfigPath
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to find home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}
	return path, nil
}

// LoadConfig reads the installer config. A missing file yields an empty config
// (no error); a malformed file is an error so a typo is surfaced to the user.
func LoadConfig(configPath string) (*Config, error) {
	path, err := ResolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	return &cfg, nil
}

// SaveConfig writes the installer config with 0600 permissions since it may
// contain a GitHub token.
func SaveConfig(configPath string, cfg *Config) error {
	path, err := ResolveConfigPath(configPath)
	if err != nil {
		return err
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
