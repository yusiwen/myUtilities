package svcreg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	DBPath string `json:"db_path"`
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

func LoadConfig(configPath string) (*Config, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				Host:   "0.0.0.0",
				Port:   30100,
				DBPath: "~/.config/mu/svcreg.db",
			}, nil
		}
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 30100
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "~/.config/mu/svcreg.db"
	}
	return &cfg, nil
}
