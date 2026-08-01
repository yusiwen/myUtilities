package scip

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func setupGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("add", "-A")
	run("commit", "-qm", "init")
	return dir
}

func TestChangedFiles(t *testing.T) {
	dir := setupGitRepo(t, map[string]string{
		"go.mod": "module probe\n",
		"a.go":   "package main\n",
	})

	if got := changedFiles(dir); len(got) != 0 {
		t.Fatalf("expected no changed files on a clean tree, got %v", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc f(){}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := changedFiles(dir)
	seen := map[string]bool{}
	for _, f := range got {
		seen[f] = true
	}
	if !seen["a.go"] || !seen["b.go"] {
		t.Fatalf("expected a.go and b.go in changed files, got %v", got)
	}
}

func TestChangedFilesRename(t *testing.T) {
	dir := setupGitRepo(t, map[string]string{
		"go.mod": "module probe\n",
		"a.go":   "package main\n",
	})
	// git mv produces a real detected rename: porcelain -z emits
	// "R  new\0old\0". The new path must be included and the old path skipped.
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("mv", "a.go", "c.go")

	got := changedFiles(dir)
	if !contains(got, "c.go") {
		t.Fatalf("expected renamed target c.go in changed files, got %v", got)
	}
	if contains(got, "a.go") {
		t.Fatalf("expected old path a.go to be excluded from changed files, got %v", got)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func TestFileMatchesIndexer(t *testing.T) {
	ix, ok := LookupLang("go")
	if !ok {
		t.Fatal("go indexer missing from registry")
	}
	cases := map[string]bool{
		"main.go":           true,
		"core/x/util.go":    true,
		"go.mod":            true,
		"go.sum":            true,
		"README.md":         false,
		"assets/app.js":     false,
		"core/main.go.tmpl": false, // *.go glob does not match the .go.tmpl suffix
	}
	for name, want := range cases {
		if got := fileMatchesIndexer(name, ix); got != want {
			t.Errorf("fileMatchesIndexer(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIndexStaleForLang(t *testing.T) {
	dir := setupGitRepo(t, map[string]string{
		"go.mod": "module probe\n",
		"a.go":   "package main\n",
	})
	ix, _ := LookupLang("go")

	cache := filepath.Join(dir, "cache.scip")
	if err := os.WriteFile(cache, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatal(err)
	}

	// No changed source files → the index is never stale.
	if indexStaleForLang(dir, ix, cache) {
		t.Fatal("expected not stale when nothing changed")
	}

	// Modify a.go after the index → stale.
	time.Sleep(5 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc f(){}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !indexStaleForLang(dir, ix, cache) {
		t.Fatal("expected stale when a.go is newer than the index")
	}

	// Touching only a non-matching file must NOT make it stale.
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	if !indexStaleForLang(dir, ix, cache) {
		t.Fatal("still expected stale (a.go still newer)")
	}

	// Bring the index mtime past the sources → not stale.
	later := time.Now()
	if err := os.Chtimes(cache, later, later); err != nil {
		t.Fatal(err)
	}
	if indexStaleForLang(dir, ix, cache) {
		t.Fatal("expected not stale when the index is newer")
	}

	// Missing index file → stale.
	if !indexStaleForLang(dir, ix, filepath.Join(dir, "missing.scip")) {
		t.Fatal("expected missing index to be stale")
	}
}

func TestIndexStaleForDeletion(t *testing.T) {
	dir := setupGitRepo(t, map[string]string{
		"go.mod": "module probe\n",
		"a.go":   "package main\n",
	})
	ix, _ := LookupLang("go")

	cache := filepath.Join(dir, "cache.scip")
	later := time.Now()
	if err := os.WriteFile(cache, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(cache, later, later); err != nil {
		t.Fatal(err)
	}

	// Deleting a matching source file must invalidate the index even though
	// there is nothing left to stat (the stale index still references it).
	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}
	if !indexStaleForLang(dir, ix, cache) {
		t.Fatal("expected a deleted source file to make the index stale")
	}
}
