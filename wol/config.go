package wol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func SetConfigValue(cfg *WOLConfig, key, value string) error {
	switch strings.ToLower(key) {
	case "server":
		cfg.Server = value
	case "interface":
		cfg.Interface = value
	case "db-path":
		cfg.DBPath = value
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid port %q: %v", value, err)
		}
		cfg.Port = port
	case "token":
		cfg.Token = value
	case "hostname":
		cfg.Hostname = value
	default:
		return fmt.Errorf("unknown config key: %q (valid: server, interface, db-path, port, token, hostname)", key)
	}
	return nil
}

func GetConfigValue(cfg *WOLConfig, key string) (string, bool) {
	switch strings.ToLower(key) {
	case "server":
		return cfg.Server, true
	case "interface":
		return cfg.Interface, true
	case "db-path":
		return cfg.DBPath, true
	case "port":
		return strconv.Itoa(cfg.Port), true
	case "token":
		return cfg.Token, true
	case "hostname":
		return cfg.Hostname, true
	default:
		return "", false
	}
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
