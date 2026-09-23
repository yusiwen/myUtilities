package k8s

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveIndexUsesRestrictivePermissions covers the secret-bearing index: it
// stores raw kubeconfig text (client keys, tokens), so it must be 0600 even
// when it already exists with looser bits (a --config-dir can point at a
// world-readable directory).
func TestSaveIndexUsesRestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	SetConfigDir(dir)
	t.Cleanup(func() { SetConfigDir("") })

	idx := &ConfigIndex{Active: "a", Configs: map[string]string{"a": "apiVersion: v1\n"}}
	if err := SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "kubeconfigs.yaml")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}

	// Rewriting a pre-existing world-readable file must tighten it again.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveIndex(idx); err != nil {
		t.Fatal(err)
	}
	fi, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode after rewrite = %o, want 600", got)
	}
}
