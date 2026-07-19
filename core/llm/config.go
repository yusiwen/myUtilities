package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

const configDir = "~/.config/mu"

func configFilePath(appName string) (string, error) {
	raw := filepath.Join(configDir, appName+".json")
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

func LoadConfig(appName string) (*Config, error) {
	cfg := &Config{
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o-mini",
	}

	path, err := configFilePath(appName)
	if err != nil {
		return nil, err
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

func SaveConfig(appName string, cfg *Config) error {
	path, err := configFilePath(appName)
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
