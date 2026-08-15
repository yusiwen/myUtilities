package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the fleet module configuration persisted at
// ~/.config/mu/fleet-config.json (file mode 0600 when it contains a token).
type Config struct {
	Server       string   `json:"server"`
	Token        string   `json:"token"`
	Hostname     string   `json:"hostname"`
	Groups       []string `json:"groups"`
	PollInterval int      `json:"poll_interval"` // seconds
	Port         int      `json:"port"`
	DBPath       string   `json:"db_path"`
	DataDir      string   `json:"data_dir"`
}

func defaultConfig() *Config {
	return &Config{
		Server:       "http://localhost:8890",
		PollInterval: 5,
		Port:         8890,
		DBPath:       "~/.cache/mu/fleet/fleet.db",
		DataDir:      "~/.cache/mu/fleet",
	}
}

// LoadConfig loads the fleet config from path. A missing file yields the
// defaults, and "~" in paths is expanded.
func LoadConfig(path string) (*Config, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[2:])
	}

	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// PollIntervalDuration returns the configured poll interval.
func (c *Config) PollIntervalDuration() time.Duration {
	if c.PollInterval <= 0 {
		return 5 * time.Second
	}
	return time.Duration(c.PollInterval) * time.Second
}

// Resolve expands "~" in path-like fields.
func (c *Config) Resolve() *Config {
	if strings.HasPrefix(c.DBPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			c.DBPath = filepath.Join(home, c.DBPath[2:])
		}
	}
	if strings.HasPrefix(c.DataDir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			c.DataDir = filepath.Join(home, c.DataDir[2:])
		}
	}
	return c
}
