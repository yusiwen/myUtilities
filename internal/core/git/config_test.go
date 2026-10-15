package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yusiwen/myUtilities/internal/core/llm"
)

// An existing git-config.json with inline providers must keep loading exactly as
// before, and a read must never rewrite the file.
func TestLegacyGitConfigLoadsAndIsNotRewritten(t *testing.T) {
	content := `{
  "providers": [
    {"name": "fast", "base_url": "https://fast/v1", "api_key": "sk-f", "model": "m"}
  ],
  "commit": {"provider": "fast", "lang": "en"},
  "review": {"provider": ["fast", "backup"], "lang": "cn"}
}`

	dir := t.TempDir()
	path := filepath.Join(dir, "git-config.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	gc, err := LoadGitConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadGitConfigFrom: %v", err)
	}
	if len(gc.Providers) != 1 || gc.Providers[0].Name != "fast" {
		t.Fatalf("providers = %#v, want the inline 'fast' provider", gc.Providers)
	}
	if gc.Source != "git-config.json" {
		t.Errorf("Source = %q, want git-config.json", gc.Source)
	}
	if len(gc.Commit.Provider) != 1 || gc.Commit.Provider[0] != "fast" {
		t.Errorf("commit provider = %#v, want [fast]", gc.Commit.Provider)
	}
	if len(gc.Review.Provider) != 2 {
		t.Errorf("review provider = %#v, want the two-name fallback chain", gc.Review.Provider)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(after) != content {
		t.Error("loading rewrote the config file; a read must never migrate silently")
	}
}

func TestGitConfigReferences(t *testing.T) {
	gc := &GitConfig{
		Commit: ModuleConfig{Provider: llm.MultiProvider{"fast"}},
		Review: ModuleConfig{Provider: llm.MultiProvider{"fast", "backup"}},
	}

	refs := gc.References()
	if len(refs) != 2 {
		t.Fatalf("References() = %#v, want one entry per module", refs)
	}
	if refs[0].Module != "commit" || refs[1].Module != "review" {
		t.Errorf("module names = %q, %q; want commit, review", refs[0].Module, refs[1].Module)
	}
	if !refs[1].Names.Contains("backup") {
		t.Errorf("review reference = %#v, want it to contain backup", refs[1].Names)
	}

	// An empty list must not be reported as a dependency.
	gc2 := &GitConfig{}
	for _, r := range gc2.References() {
		if len(r.Names) != 0 {
			t.Errorf("empty config reported a reference: %#v", r)
		}
	}
	if got := strings.Join(gc2.References()[0].Names.Names(), ","); got != "" {
		t.Errorf("empty commit provider names = %q, want empty", got)
	}
}
