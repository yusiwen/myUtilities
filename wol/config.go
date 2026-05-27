package wol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type WOLConfig struct {
	Server    string `json:"server"`
	DBPath    string `json:"db_path"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
	Interface string `json:"interface"`
	Hostname  string `json:"hostname"`
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

func LoadConfig(configPath string) (*WOLConfig, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &WOLConfig{
				DBPath: "~/.config/mu/bolt.db",
				Port:   8080,
			}, nil
		}
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg WOLConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "~/.config/mu/bolt.db"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	return &cfg, nil
}

func saveConfig(configPath string, cfg *WOLConfig) error {
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
