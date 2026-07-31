package es

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func ResolveConfigPath(raw string) (string, error) {
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

func LoadConfig(configPath string) (*Config, error) {
	path, err := ResolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Host: "http://localhost:9200"}, nil
		}
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	if cfg.Host == "" {
		cfg.Host = "http://localhost:9200"
	}
	return &cfg, nil
}

func SaveConfig(configPath string, cfg *Config) error {
	path, err := ResolveConfigPath(configPath)
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

func MaskedConfig(cfg *Config) *Config {
	masked := *cfg
	if masked.Password != "" {
		masked.Password = "***"
	}
	return &masked
}
