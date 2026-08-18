package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Retention       string `json:"retention,omitempty"`
	Interval        string `json:"collect_interval,omitempty"`
	CompactInterval string `json:"compact_interval,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	ServerURL       string `json:"server_url,omitempty"`
	DataDir         string `json:"data_dir,omitempty"`
	DebugLog        bool   `json:"debug_log,omitempty"`
}

// DefaultCompactInterval is used when compact_interval is unset or invalid.
const DefaultCompactInterval = 24 * time.Hour

// ResolveConfigDir returns the effective config directory: the given dir with
// a leading ~/ expanded, or ~/.config/mu when empty.
func ResolveConfigDir(configDir string) string {
	if configDir != "" {
		return ExpandTilde(configDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mu")
}

// FormatDuration renders a duration compactly: days, hours, or minutes when
// whole, otherwise Go's default string form.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
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

// ExpandTilde expands a leading "~/" to the user's home directory.
func ExpandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// LoadConfigDir loads metrics-config.json from dir. An empty dir falls back to
// the default config path under the user's home.
func LoadConfigDir(dir string) (*Config, error) {
	if dir == "" {
		return LoadConfig("")
	}
	return LoadConfig(filepath.Join(ExpandTilde(dir), "metrics-config.json"))
}

// ResolveDBPath picks the metrics DB file path. Priority: --db-path flag >
// config data_dir > --config-dir > the legacy default under the user's home.
func ResolveDBPath(configDir, cfgDataDir, dbPath string) (string, error) {
	if dbPath != "" {
		return ExpandTilde(dbPath), nil
	}
	if cfgDataDir != "" {
		return filepath.Join(ExpandTilde(cfgDataDir), "metrics.db"), nil
	}
	if configDir != "" {
		return filepath.Join(ExpandTilde(configDir), "metrics.db"), nil
	}
	dir, err := DefaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "metrics.db"), nil
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

// ResolveRetention resolves the retention window. Semantics:
//
//	flag ""   -> use the configured value (cfgVal)
//	flag "0"  -> 0 (keep forever), explicitly overriding cfgVal
//	flag "Nd" or a Go duration -> that value
//	invalid flag -> 0 (keep forever)
func ResolveRetention(flagVal, cfgVal string) time.Duration {
	if flagVal != "" {
		return ParseRetention(flagVal)
	}
	return ParseRetention(cfgVal)
}

// ResolveCompactInterval returns the configured compact interval, falling back
// to DefaultCompactInterval when unset, invalid, or non-positive.
func ResolveCompactInterval(cfgVal string) time.Duration {
	d, err := time.ParseDuration(cfgVal)
	if err != nil || d <= 0 {
		return DefaultCompactInterval
	}
	return d
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
