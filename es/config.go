package es

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ESConfig struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func resolveConfigPath(raw string) (string, error) {
	path := raw
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to find home directory: %v", err)
		}
		path = filepath.Join(home, path[2:])
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %v", err)
	}
	return path, nil
}

func loadConfig(configPath string) (*ESConfig, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ESConfig{Host: "http://localhost:9200"}, nil
		}
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg ESConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	if cfg.Host == "" {
		cfg.Host = "http://localhost:9200"
	}
	return &cfg, nil
}

func saveConfig(configPath string, cfg *ESConfig) error {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}
	return nil
}

func maskedPassword(cfg *ESConfig) *ESConfig {
	masked := *cfg
	if masked.Password != "" {
		masked.Password = "***"
	}
	return &masked
}
