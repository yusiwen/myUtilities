package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// A config that defines named providers but no "default" and no provider[]
// must NOT be migrated to a nonexistent "default" reference.
func TestLoadConfigFromPath_NoDefaultProvider(t *testing.T) {
	path := writeTemp(t, "ask-config.json", `{
		"providers": [
			{"name": "fast", "base_url": "https://fast/v1", "api_key": "sk-f", "model": "m"},
			{"name": "backup", "base_url": "https://backup/v1", "api_key": "sk-b", "model": "m"}
		]
	}`)

	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}
	if len(cfg.Provider) != 0 {
		t.Fatalf("expected empty Provider reference, got %#v", cfg.Provider)
	}
}

// Legacy flat fields should still migrate into a usable "default" provider.
func TestLoadConfigFromPath_LegacyMigratesToDefault(t *testing.T) {
	path := writeTemp(t, "ask-config.json", `{
		"base_url": "https://api.openai.com/v1",
		"api_key": "sk-legacy",
		"model": "gpt-4o-mini"
	}`)

	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "default" {
		t.Fatalf("expected a single 'default' provider, got %#v", cfg.Providers)
	}
	if len(cfg.Provider) != 1 || cfg.Provider[0] != "default" {
		t.Fatalf("expected Provider reference ['default'], got %#v", cfg.Provider)
	}
}

func TestParseProviderList(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"", nil, false},
		{"fast", []string{"fast"}, false},
		{" fast , backup ", []string{"fast", "backup"}, false},
		{"default,backup", []string{"default", "backup"}, false},
		{",,", nil, true},
		{"  ,  ", nil, true},
	}
	for _, c := range cases {
		got, err := ParseProviderList(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseProviderList(%q): expected error, got %#v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProviderList(%q): unexpected error %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("ParseProviderList(%q) = %#v, want %#v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ParseProviderList(%q) = %#v, want %#v", c.in, got, c.want)
				break
			}
		}
	}
}

// SaveConfig and LoadConfig must agree on the same path so a round-trip works.
func TestSaveLoadPathConsistency(t *testing.T) {
	path := writeTemp(t, "ask-config.json", `{"api_key":"sk-roundtrip"}`)
	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.SearchAPIKey = "BSA-x"
	if err := SaveConfigToPath(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	cfg2, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg2.SearchAPIKey != "BSA-x" {
		t.Fatalf("round-trip lost SearchAPIKey, got %q", cfg2.SearchAPIKey)
	}
	if len(cfg2.Providers) != 1 || cfg2.Providers[0].Name != "default" {
		t.Fatalf("expected migrated default provider, got %#v", cfg2.Providers)
	}
}
