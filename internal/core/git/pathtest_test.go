package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGitConfigExplicitPathRoundTrip covers the --config plumbing: a custom path
// must be written and read back (0600), independently of ~/.config/mu.
func TestGitConfigExplicitPathRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom", "git-config.json")
	want := &GitConfig{
		Providers: []Provider{{Name: "p1", BaseURL: "https://example.invalid/v1", APIKey: "k"}},
		Commit:    ModuleConfig{Provider: MultiProvider{"p1"}, Lang: "en"},
	}
	if err := SaveGitConfigTo(path, want); err != nil {
		t.Fatalf("SaveGitConfigTo: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}

	got, err := LoadGitConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadGitConfigFrom: %v", err)
	}
	if len(got.Providers) != 1 || got.Providers[0].Name != "p1" {
		t.Fatalf("providers = %+v, want the saved provider", got.Providers)
	}
	if got.Commit.Lang != "en" {
		t.Fatalf("commit lang = %q, want en", got.Commit.Lang)
	}
}

// TestLoadGitConfigFromMissingPath checks the empty-config contract for a path
// that does not exist yet (first run with --config).
func TestLoadGitConfigFromMissingPath(t *testing.T) {
	got, err := LoadGitConfigFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadGitConfigFrom: %v", err)
	}
	if got == nil || len(got.Providers) != 0 {
		t.Fatalf("expected an empty config, got %+v", got)
	}
}
