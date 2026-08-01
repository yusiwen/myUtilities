package scip

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These tests verify every query API in core/scip against a real SCIP index
// built from this repository's own Go code. They require a network connection
// and the `go` toolchain on the first run (to auto-download scip-go); if the
// index cannot be built, the tests skip gracefully.

var (
	onceReal sync.Once
	realSet  *IndexSet
	realRoot string
	realErr  error
)

func gitToplevel() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// realIndex builds (or reuses) the SCIP index for this repo once per test run.
func realIndex(t *testing.T) *IndexSet {
	t.Helper()
	onceReal.Do(func() {
		realRoot, realErr = gitToplevel()
		if realErr != nil {
			return
		}
		// A dirty working tree invalidates the per-commit cache, so force a
		// regeneration to guarantee the queries match the current source.
		realSet, realErr = EnsureIndex(EnsureOptions{
			RepoRoot:    realRoot,
			AutoInstall: true,
			Force:       repoDirty(realRoot),
			Out:         io.Discard,
		})
	})
	if realErr != nil {
		t.Skipf("skipping SCIP integration tests (index build failed): %v", realErr)
	}
	if realSet == nil || len(realSet.Langs()) == 0 {
		t.Skip("skipping SCIP integration tests: no languages detected in repo")
	}
	return realSet
}

// repoLine returns the 1-based line of the first line in a repo-relative file
// that contains pattern, failing the test if it is not found.
func repoLine(t *testing.T, relPath, pattern string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(realRoot, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, pattern) {
			return i + 1
		}
	}
	t.Fatalf("pattern %q not found in %s", pattern, relPath)
	return 0
}

// TestIntegrationFindDefinitionOnDefLine verifies that FindDefinition resolves
// the symbol at its own definition line.
func TestIntegrationFindDefinitionOnDefLine(t *testing.T) {
	set := realIndex(t)
	defLine := repoLine(t, "core/scip/runner.go", "func EnsureIndex(")

	defs, err := set.FindDefinition("core/scip/runner.go", defLine)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range defs {
		if d.Path == "core/scip/runner.go" && d.Line == defLine && d.IsDef &&
			strings.Contains(d.Symbol, "EnsureIndex") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected EnsureIndex definition at runner.go:%d, got %+v", defLine, defs)
	}
}

// TestIntegrationFindDefinitionFromUsage verifies that a call site resolves to
// the cross-package definition of the called symbol.
func TestIntegrationFindDefinitionFromUsage(t *testing.T) {
	set := realIndex(t)
	defLine := repoLine(t, "core/scip/runner.go", "func EnsureIndex(")
	usageLine := repoLine(t, "git/review.go", "scip.EnsureIndex(")

	defs, err := set.FindDefinition("git/review.go", usageLine)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range defs {
		if d.Path == "core/scip/runner.go" && d.Line == defLine && d.IsDef &&
			strings.Contains(d.Symbol, "EnsureIndex") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected usage at git/review.go:%d to resolve to runner.go:%d, got %+v",
			usageLine, defLine, defs)
	}
}

// TestIntegrationFindReferencesCrossFile verifies that references are resolved
// across the whole repository, not just within the querying file.
func TestIntegrationFindReferencesCrossFile(t *testing.T) {
	set := realIndex(t)
	defLine := repoLine(t, "core/scip/runner.go", "func EnsureIndex(")

	refs, err := set.FindReferences("core/scip/runner.go", defLine)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 2 {
		t.Fatalf("expected multiple references, got %d: %+v", len(refs), refs)
	}
	// The definition location itself must be present.
	var hasDef, crossFile bool
	for _, r := range refs {
		if r.IsDef && r.Path == "core/scip/runner.go" && strings.Contains(r.Symbol, "EnsureIndex") {
			hasDef = true
		}
		if r.Path != "core/scip/runner.go" {
			crossFile = true
		}
	}
	if !hasDef {
		t.Fatalf("expected the definition among references, got %+v", refs)
	}
	if !crossFile {
		t.Fatalf("expected at least one cross-file usage of EnsureIndex, got %+v", refs)
	}
}

// TestIntegrationSymbolInfo verifies hover-style information (kind/signature).
func TestIntegrationSymbolInfo(t *testing.T) {
	set := realIndex(t)
	defLine := repoLine(t, "core/scip/runner.go", "func EnsureIndex(")

	info, err := set.SymbolInfoAt("core/scip/runner.go", defLine)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatalf("no symbol info at runner.go:%d", defLine)
	}
	if info.Kind != "Function" {
		t.Errorf("expected kind Function, got %q", info.Kind)
	}
	if !strings.Contains(info.Signature, "EnsureIndex") {
		t.Errorf("expected EnsureIndex in signature, got %q", info.Signature)
	}
	if info.DisplayName != "EnsureIndex" {
		t.Errorf("expected display name EnsureIndex, got %q", info.DisplayName)
	}
}

// TestIntegrationEnclosingDefLine verifies that a line inside a function body
// maps back to the enclosing function's definition line.
func TestIntegrationEnclosingDefLine(t *testing.T) {
	set := realIndex(t)
	defLine := repoLine(t, "core/scip/runner.go", "func EnsureIndex(")
	bodyLine := repoLine(t, "core/scip/runner.go", "project := repoProject(opts.RepoRoot)")

	ix, ok := set.IndexFor("core/scip/runner.go")
	if !ok {
		t.Fatal("runner.go not in index")
	}
	if got := ix.EnclosingDefLine("core/scip/runner.go", bodyLine); got != defLine {
		t.Fatalf("expected enclosing def line %d for body line %d, got %d", defLine, bodyLine, got)
	}
}

// TestIntegrationSymbolsInRange verifies the range query returns symbols
// occurring within an inclusive line range.
func TestIntegrationSymbolsInRange(t *testing.T) {
	set := realIndex(t)
	defLine := repoLine(t, "core/scip/runner.go", "func EnsureIndex(")

	infos, err := set.SymbolsInRange("core/scip/runner.go", defLine, defLine+3)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, info := range infos {
		if info.DisplayName == "EnsureIndex" || strings.Contains(info.Symbol, "EnsureIndex") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected EnsureIndex in range [%d,%d], got %+v", defLine, defLine+3, infos)
	}
}

// TestIntegrationIndexForRouting verifies file → index routing and that
// missing files report ErrNoIndex.
func TestIntegrationIndexForRouting(t *testing.T) {
	set := realIndex(t)

	if _, ok := set.IndexFor("core/scip/runner.go"); !ok {
		t.Fatal("expected runner.go to route to the go index")
	}
	if _, ok := set.IndexFor("nonexistent.go"); ok {
		t.Fatal("did not expect nonexistent.go to route to an index")
	}
	if _, err := set.FindReferences("nonexistent.go", 1); err == nil {
		t.Fatal("expected ErrNoIndex for a file outside the index")
	}
}

// TestIntegrationOffByOne verifies that querying a line with no occurrences
// returns an empty result, not a shifted one.
func TestIntegrationOffByOne(t *testing.T) {
	set := realIndex(t)
	// Query a line far beyond the file length — must be empty, not error.
	defs, err := set.FindDefinition("core/scip/runner.go", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 0 {
		t.Fatalf("expected no definitions beyond EOF, got %+v", defs)
	}
}
