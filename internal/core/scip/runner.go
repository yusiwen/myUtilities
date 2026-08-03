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

	"github.com/briandowns/spinner"
	"github.com/yusiwen/myUtilities/internal/core/term"
	xterm "golang.org/x/term"
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
		reason := ""
		if !opts.Force && fileExists(path) {
			// Commit-cached indexes are immutable. A dirty-tree "working" index
			// is only reusable while no matching source file changed after it
			// was generated.
			if commit == workingCommit && indexStaleForLang(opts.RepoRoot, ix, path) {
				reason = fmt.Sprintf("%s index is stale; rebuilding ...", LangDisplay(lang))
			} else {
				needsGenerate = false
				tc.debugf("reusing cached %s index: %s", lang, path)
			}
		} else if opts.Force {
			reason = fmt.Sprintf("Rebuilding %s index (--refresh-scip)", LangDisplay(lang))
		}

		if needsGenerate {
			if opts.Force {
				os.Remove(path)
			}
			bin, err := tc.Install(ix)
			if err != nil {
				if autoInstall {
					return nil, indexErr(ix, fmt.Errorf("install %s indexer: %w", lang, err))
				}
				tc.logf(term.Faint("Skipping %s: %s"), LangDisplay(lang), err)
				continue
			}
			if err := preflightRequires(ix); err != nil {
				if autoInstall {
					return nil, indexErr(ix, err)
				}
				tc.logf(term.Faint("Skipping %s: %s"), LangDisplay(lang), err)
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
				return generate(tc, ix, bin, opts.RepoRoot, path, reason)
			}); err != nil {
				tc.logf(term.Faint("Failed to build %s index: %s"), LangDisplay(lang), err)
				if autoInstall {
					return nil, indexErr(ix, err)
				}
				continue
			}
		}

		loaded, err := Load(path)
		if err != nil {
			tc.logf(term.Faint("Failed to load %s index: %s"), LangDisplay(lang), err)
			continue
		}
		set.Add(lang, loaded)
	}

	return set, nil
}

// IndexError describes a failed index build for a language. Hard marks an
// unrecoverable failure that should abort the review instead of silently
// falling back to text tools (e.g. scip-java runs a real build).
type IndexError struct {
	Lang string
	Err  error
	Hard bool
}

func (e *IndexError) Error() string {
	return fmt.Sprintf("failed to build %s index: %v", e.Lang, e.Err)
}

func (e *IndexError) Unwrap() error { return e.Err }

// indexErr wraps a per-language indexing failure, marking it hard when the
// indexer's FailHard flag is set.
func indexErr(ix *Indexer, err error) error {
	return &IndexError{Lang: ix.Lang, Err: err, Hard: ix.FailHard}
}

// generate runs the indexer binary and writes the index to outPath. reason is
// an optional visible line describing why the index is being (re)built.
func generate(tc *Toolchain, ix *Indexer, bin, repoRoot, outPath, reason string) error {
	start := time.Now()
	text := "Building " + LangDisplay(ix.Lang) + " index ..."
	if reason != "" {
		text = reason
	}
	spin := newIndexSpinner(tc, text)
	if spin != nil {
		spin.Start()
	} else {
		tc.logf(term.Faint("%s"), text)
	}

	args := buildArgs(tc, ix, outPath)
	res := runIndexer(tc, bin, args, repoRoot, outPath)

	if spin != nil {
		spin.Stop()
	}
	if res.Success {
		tc.logf(term.Faint("%s index ready (%s)"), LangDisplay(ix.Lang), formatDuration(time.Since(start)))
		return nil
	}

	var sb strings.Builder
	sb.WriteString(ix.BinaryName + " indexer failed")
	if res.Err != nil {
		sb.WriteString(" (" + res.Err.Error() + ")")
	}
	if res.Log != "" {
		sb.WriteString(": " + extractError(res.Log))
	}
	if res.LogPath != "" {
		sb.WriteString(fmt.Sprintf("\nFull indexer log kept at: %s", res.LogPath))
	}
	return errors.New(sb.String())
}

// LangDisplay returns the display name for a language (e.g. "go" → "Go").
func LangDisplay(lang string) string {
	if lang == "" {
		return lang
	}
	if n, ok := langDisplayNames[lang]; ok {
		return n
	}
	return strings.ToUpper(lang[:1]) + lang[1:]
}

var langDisplayNames = map[string]string{
	"go": "Go", "java": "Java", "rust": "Rust",
	"typescript": "TypeScript", "c": "C",
}

// formatDuration renders a duration compactly, e.g. "850ms", "2.3s", "1m5s".
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	default:
		return d.Round(time.Second).String()
	}
}

// buildArgs constructs the indexer invocation from the data-driven fields:
// <Prefix> <OutputFlag> <outPath> [<QuietFlag> when non-verbose] <Trailing>.
func buildArgs(tc *Toolchain, ix *Indexer, outPath string) []string {
	args := append([]string{}, ix.Prefix...)
	args = append(args, ix.OutputFlag, outPath)
	if !tc.Verbose && ix.QuietFlag != "" {
		args = append(args, ix.QuietFlag)
	}
	args = append(args, ix.Trailing...)
	return args
}

// newIndexSpinner returns a spinner writing to the toolchain writer when it is
// an interactive terminal and verbose mode is off (verbose streams output live
// and would interleave with the animation).
func newIndexSpinner(tc *Toolchain, text string) *spinner.Spinner {
	if tc.Verbose {
		return nil
	}
	f, ok := tc.Out.(*os.File)
	if !ok || !xterm.IsTerminal(int(f.Fd())) {
		return nil
	}
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + text
	s.Writer = tc.Out
	s.Color("fgHiWhite")
	return s
}

// IndexResult captures the outcome of an indexer invocation.
type IndexResult struct {
	Success bool
	Err     error
	Log     string
	LogPath string
}

// runIndexer executes an indexer, capturing its stdout/stderr to a temp file
// (streamed directly to the toolchain writer in verbose mode) and reporting
// success based on the exit code and the presence of the produced index file.
//
// On failure the temp file is retained for diagnostics and its path is
// returned; on success it is removed.
func runIndexer(tc *Toolchain, bin string, args []string, repoRoot, outPath string) *IndexResult {
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot

	if tc.Verbose {
		// Verbose: stream everything live, no temp file.
		cmd.Stdout, cmd.Stderr = tc.Out, tc.Out
		err := cmd.Run()
		return &IndexResult{Success: err == nil && fileExists(outPath), Err: err}
	}

	// Default: capture all indexer output to a temp file.
	tmp, err := os.CreateTemp("", "scip-index-*")
	if err != nil {
		return &IndexResult{Err: err}
	}
	cmd.Stdout, cmd.Stderr = tmp, tmp

	runErr := cmd.Run()
	tmp.Close()
	logData, _ := os.ReadFile(tmp.Name())
	success := runErr == nil && fileExists(outPath)

	if !success {
		return &IndexResult{
			Success: false,
			Err:     runErr,
			Log:     strings.TrimSpace(string(logData)),
			LogPath: tmp.Name(),
		}
	}
	os.Remove(tmp.Name())
	return &IndexResult{Success: true}
}

// extractError parses an indexer log and returns the most relevant error lines
// (matching error-like keywords), falling back to the tail of the log.
func extractError(log string) string {
	lines := strings.Split(log, "\n")
	var hits []string
	seen := map[string]bool{}
	for _, ln := range lines {
		low := strings.ToLower(ln)
		for _, kw := range []string{"error", "failed", "exception", "build failure", "caused by", "fatal"} {
			if strings.Contains(low, kw) && !seen[low] {
				seen[low] = true
				hits = append(hits, strings.TrimSpace(ln))
				break
			}
		}
		if len(hits) >= 15 {
			break
		}
	}
	if len(hits) > 0 {
		return strings.Join(hits, "\n")
	}
	if len(lines) > 15 {
		return strings.Join(lines[len(lines)-15:], "\n")
	}
	return log
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
