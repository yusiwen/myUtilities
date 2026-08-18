package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(cfg *Config, path string) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func TestResolveDBPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// --db-path wins over everything.
	if got, _ := ResolveDBPath("/cfg", "", "/abs/metrics.db"); got != "/abs/metrics.db" {
		t.Fatalf("db-path override: got %s", got)
	}

	// config data_dir beats config-dir.
	if got, _ := ResolveDBPath("/cfg", "/data", ""); got != filepath.Join("/data", "metrics.db") {
		t.Fatalf("data_dir: got %s", got)
	}

	// config-dir is the default DB directory when nothing else is set.
	if got, _ := ResolveDBPath("/etc/mu", "", ""); got != filepath.Join("/etc/mu", "metrics.db") {
		t.Fatalf("config-dir default: got %s", got)
	}

	// Legacy fallback when no config-dir is given.
	if got, _ := ResolveDBPath("", "", ""); got != filepath.Join(home, ".local", "share", "mu", "metrics", "metrics.db") {
		t.Fatalf("legacy default: got %s", got)
	}

	// Tilde expansion on the flag and config values.
	if got, _ := ResolveDBPath("~/cfg", "", ""); got != filepath.Join(home, "cfg", "metrics.db") {
		t.Fatalf("tilde config-dir: got %s", got)
	}
	if got, _ := ResolveDBPath("", "~/data", ""); got != filepath.Join(home, "data", "metrics.db") {
		t.Fatalf("tilde data_dir: got %s", got)
	}
	if got, _ := ResolveDBPath("", "", "~/x/db.db"); got != filepath.Join(home, "x", "db.db") {
		t.Fatalf("tilde db-path: got %s", got)
	}
}

func TestLoadConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DataDir: "/custom/data", Retention: "7d"}
	if err := writeConfig(cfg, filepath.Join(dir, "metrics-config.json")); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfigDir(dir)
	if err != nil {
		t.Fatalf("LoadConfigDir: %v", err)
	}
	if got.DataDir != "/custom/data" || got.Retention != "7d" {
		t.Fatalf("unexpected config: %+v", got)
	}

	// Missing config file in dir -> empty config, no error.
	empty, err := LoadConfigDir(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("LoadConfigDir missing: %v", err)
	}
	if empty.DataDir != "" {
		t.Fatalf("expected empty config, got %+v", empty)
	}
}

func TestResolveRetention(t *testing.T) {
	cases := []struct {
		flag, cfg string
		want      time.Duration
	}{
		{"", "7d", 7 * 24 * time.Hour},  // empty flag inherits config
		{"", "", 0},                     // nothing set -> forever
		{"0", "7d", 0},                  // explicit 0 overrides config (forever)
		{"7d", "", 7 * 24 * time.Hour},  // flag wins
		{"30m", "7d", 30 * time.Minute}, // Go duration
		{"abc", "7d", 0},                // invalid flag -> 0 (forever)
	}
	for _, c := range cases {
		got := ResolveRetention(c.flag, c.cfg)
		if got != c.want {
			t.Errorf("ResolveRetention(%q, %q) = %v, want %v", c.flag, c.cfg, got, c.want)
		}
	}
}

func TestResolveCompactInterval(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", DefaultCompactInterval},
		{"1h", time.Hour},
		{"5s", 5 * time.Second},
		{"0", DefaultCompactInterval},
		{"abc", DefaultCompactInterval},
	}
	for _, c := range cases {
		got := ResolveCompactInterval(c.in)
		if got != c.want {
			t.Errorf("ResolveCompactInterval(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
