package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultConfigPath = "~/.config/mu/commit.json"

type CommitConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

func resolveConfigPath(raw string) (string, error) {
	path := raw
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

func loadConfig() (*CommitConfig, error) {
	path, err := resolveConfigPath(defaultConfigPath)
	if err != nil {
		return nil, err
	}

	cfg := &CommitConfig{
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o-mini",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

func saveConfig(configPath string, cfg *CommitConfig) error {
	path, err := resolveConfigPath(configPath)
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
