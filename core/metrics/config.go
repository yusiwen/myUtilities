package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Retention string `json:"retention,omitempty"`
	Interval  string `json:"collect_interval,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	ServerURL string `json:"server_url,omitempty"`
	DataDir   string `json:"data_dir,omitempty"`
	DebugLog  bool   `json:"debug_log,omitempty"`
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	return filepath.Join(home, ".config", "mu", "metrics-config.json"), nil
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "mu", "metrics"), nil
}

func LoadConfig(configPath string) (*Config, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	cfg := &Config{}
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

func ResolveRetention(flagVal, cfgVal string) time.Duration {
	if flagVal != "0" {
		if d := ParseRetention(flagVal); d > 0 {
			return d
		}
	}
	return ParseRetention(cfgVal)
}

func ResolveInterval(flagVal, cfgVal string) time.Duration {
	v := flagVal
	if v == "" || v == "30s" {
		v = cfgVal
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 30 * time.Second
	}
	return d
}

func ResolveHostname(flagVal, cfgVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if cfgVal != "" {
		return cfgVal
	}
	host, _ := os.Hostname()
	return host
}
