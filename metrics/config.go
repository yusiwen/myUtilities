package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type MetricsConfig struct {
	Retention string `json:"retention,omitempty"`
	Interval  string `json:"collect_interval,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	ServerURL string `json:"server_url,omitempty"`
	DataDir   string `json:"data_dir,omitempty"`
	DebugLog  bool   `json:"debug_log,omitempty"`
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	return filepath.Join(home, ".config", "mu", "metrics-config.json"), nil
}

func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "mu", "metrics"), nil
}

func loadConfig(configPath string) (*MetricsConfig, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	cfg := &MetricsConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read metrics config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse metrics config: %w", err)
	}

	return cfg, nil
}
