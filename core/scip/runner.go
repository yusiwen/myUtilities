package scip

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yusiwen/myUtilities/core/term"
)

const workingCommit = "working"

// EnsureOptions configures EnsureIndex.
type EnsureOptions struct {
	// RepoRoot is the repository (or workspace) directory to index.
	RepoRoot string
	// CacheDir overrides the index/tool cache root. Empty → default.
	CacheDir string
	// Force regenerates the index even when a cache entry exists.
	Force bool
	// Token is passed to the GitHub API for release downloads.
	Token string
	// AutoInstall allows downloading missing indexer binaries.
	AutoInstall bool
	// Verbose enables progress/debug output.
	Verbose bool
	// Out receives progress output (defaults to stderr).
	Out io.Writer
}

// IndexStore locates cached indexes for a project.
type IndexStore struct {
	root string
}

// NewIndexStore returns a store rooted at cacheDir (or the default).
func NewIndexStore(cacheDir string) *IndexStore {
	if cacheDir == "" {
		cacheDir = DefaultCacheRoot()
	}
	return &IndexStore{root: cacheDir}
}

// Root returns the store's cache root.
func (s *IndexStore) Root() string { return s.root }

// IndexPath returns the cached index path for project/lang/commit.
func (s *IndexStore) IndexPath(project, lang, commit string) string {
	if commit == "" {
		commit = workingCommit
	}
	return filepath.Join(s.root, "index", project, lang, commit+".scip")
}

// LockPath returns the lock file guarding generation for project/lang.
func (s *IndexStore) LockPath(project, lang string) string {
	return filepath.Join(s.root, "index", project, lang, ".lock")
}

// EnsureIndex detects languages in repoRoot, installs indexers if needed,
// generates (or reuses) cached indexes, and returns the loaded IndexSet.
func EnsureIndex(opts EnsureOptions) (*IndexSet, error) {
	if opts.RepoRoot == "" {
		return nil, errors.New("RepoRoot is required")
	}
	info, err := os.Stat(opts.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("repo root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repo root %q is not a directory", opts.RepoRoot)
	}
	if opts.Out == nil {
		opts.Out = os.Stderr
	}
	autoInstall := opts.AutoInstall

	project := repoProject(opts.RepoRoot)
	commit := currentCommit(opts.RepoRoot)
	dirty := repoDirty(opts.RepoRoot)
	if dirty {
		commit = workingCommit
	}

	langs, _ := DetectLangs(opts.RepoRoot)
	if len(langs) == 0 {
		return nil, nil
	}

	store := NewIndexStore(opts.CacheDir)
	tc := &Toolchain{Root: store.root, Token: opts.Token, Verbose: opts.Verbose, Out: opts.Out}
	set := NewIndexSet(opts.RepoRoot)

	for _, lang := range langs {
		ix, ok := LookupLang(lang)
		if !ok || ix.Disable {
			continue
		}

		path := store.IndexPath(project, lang, commit)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}

		needsGenerate := true
		if !opts.Force && fileExists(path) {
			// Commit-cached indexes are immutable. A dirty-tree "working" index
			// is only reusable while no matching source file changed after it
			// was generated.
			if commit == workingCommit && indexStaleForLang(opts.RepoRoot, ix, path) {
				tc.debugf("%s working index is stale, regenerating", lang)
			} else {
				needsGenerate = false
				tc.debugf("reusing cached %s index: %s", lang, path)
			}
		}

		if needsGenerate {
			if opts.Force {
				os.Remove(path)
			}
			bin, err := tc.Install(ix)
			if err != nil {
				if autoInstall {
					return nil, fmt.Errorf("install %s indexer: %w", lang, err)
				}
				tc.logf(term.Faint("skipping %s: %s"), lang, err)
				continue
			}
			if err := preflightRequires(ix); err != nil {
				if autoInstall {
					return nil, err
				}
				tc.logf(term.Faint("skipping %s: %s"), lang, err)
				continue
			}

			if err := withLock(store.LockPath(project, lang), func() error {
				// Another process may have generated it while we waited for
				// the lock; re-check before generating.
				if fileExists(path) && !opts.Force {
					if commit != workingCommit || !indexStaleForLang(opts.RepoRoot, ix, path) {
						return nil
					}
				}
				return generate(tc, ix, bin, opts.RepoRoot, path)
			}); err != nil {
				tc.logf(term.Faint("indexing %s failed: %s"), lang, err)
				if autoInstall {
					return nil, err
				}
				continue
			}
		}

		loaded, err := Load(path)
		if err != nil {
			tc.logf(term.Faint("loading %s index failed: %s"), lang, err)
			continue
		}
		set.Add(lang, loaded)
	}

	return set, nil
}

// generate runs the indexer binary and writes the index to outPath.
func generate(tc *Toolchain, ix *Indexer, bin, repoRoot, outPath string) error {
	tc.logf("Indexing %s ...", term.Bright(ix.Lang))
	args := []string{"-o", outPath, "-q"}
	if tc.Verbose {
		args = []string{"-o", outPath}
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot
	cmd.Stdout = tc.Out
	cmd.Stderr = tc.Out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s indexer: %w", ix.BinaryName, err)
	}
	if !fileExists(outPath) {
		return fmt.Errorf("%s did not produce %s", ix.BinaryName, outPath)
	}
	return nil
}

func preflightRequires(ix *Indexer) error {
	for _, r := range ix.Requires {
		if _, err := exec.LookPath(r); err != nil {
			return fmt.Errorf("%s indexer requires %q in PATH", ix.Lang, r)
		}
	}
	return nil
}

// withLock runs fn while holding an exclusive lock file, waiting for any
// concurrent generation to finish.
func withLock(lockPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			f.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for lock %s", lockPath)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

/* ─── git helpers ─── */

func runGitIn(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// repoProject returns the repository top-level directory base name.
func repoProject(repoRoot string) string {
	top := runGitIn(repoRoot, "rev-parse", "--show-toplevel")
	if top != "" {
		return filepath.Base(top)
	}
	return filepath.Base(repoRoot)
}

// currentCommit returns the short HEAD commit, or "" if not a git repo.
func currentCommit(repoRoot string) string {
	return runGitIn(repoRoot, "rev-parse", "--short", "HEAD")
}

// repoDirty reports whether the working tree has changes.
func repoDirty(repoRoot string) bool {
	out := runGitIn(repoRoot, "status", "--porcelain")
	return out != ""
}

// indexStaleForLang reports whether the cached index at cachePath is stale for
// the indexer's language: any file matching the language that was modified
// after the index was generated, or that was deleted, invalidates it. A
// missing index is always considered stale.
//
// Staleness is mtime-based, which has filesystem-granularity limits (two
// edits within one timestamp tick are not detected); on coarse filesystems
// use --refresh-scip for a guaranteed rebuild.
func indexStaleForLang(repoRoot string, ix *Indexer, cachePath string) bool {
	cacheInfo, err := os.Stat(cachePath)
	if err != nil {
		return true
	}
	return hasLanguageChanges(repoRoot, ix, cacheInfo.ModTime())
}

// hasLanguageChanges reports whether a file matching the indexer's language was
// deleted or modified after indexTime in the working tree.
func hasLanguageChanges(repoRoot string, ix *Indexer, indexTime time.Time) bool {
	for _, f := range changedFiles(repoRoot) {
		if !fileMatchesIndexer(f, ix) {
			continue
		}
		info, err := os.Stat(filepath.Join(repoRoot, f))
		if err != nil {
			// Deleted or unreadable → the stale index still contains symbols
			// for this file, so it must be regenerated.
			return true
		}
		if info.ModTime().After(indexTime) {
			return true
		}
	}
	return false
}

// fileMatchesIndexer reports whether a repo-relative path belongs to the
// indexer's language, based on its Detect signals (base-name match or glob).
func fileMatchesIndexer(relPath string, ix *Indexer) bool {
	base := path.Base(relPath)
	for _, sig := range ix.Detect {
		if isGlob(sig) {
			if ok, _ := path.Match(sig, base); ok {
				return true
			}
			continue
		}
		if base == sig {
			return true
		}
	}
	return false
}

// changedFiles returns repo-relative paths of tracked modified and untracked
// files in the working tree. Returns nil when not in a git repository.
// Uses -z (NUL-separated) output so paths with special characters are not
// C-style quoted; rename records contribute only their new path.
func changedFiles(repoRoot string) []string {
	cmd := exec.Command("git", "status", "--porcelain", "-z", "--untracked-files=all")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	parts := bytes.Split(out, []byte{0})
	var files []string
	for _, part := range parts {
		if len(part) < 4 {
			continue
		}
		if part[2] != ' ' {
			// Old-path record of a rename; the new path was already collected
			// from the preceding "XY <new>" status record.
			continue
		}
		files = append(files, string(part[3:]))
	}
	return files
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
