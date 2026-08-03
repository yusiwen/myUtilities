package git

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const MaxDiffLength = 8000

type DiffResult struct {
	Stat   string
	Diff   string
	RawLen int
}

func CheckPreflight() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH — is git installed?")
	}

	out, err := exec.Command("git", "rev-parse", "--git-dir").CombinedOutput()
	if err != nil {
		stderr := strings.TrimSpace(string(out))
		if stderr != "" {
			return fmt.Errorf("not a git repository: %s", stderr)
		}
		return fmt.Errorf("not a git repository")
	}

	return nil
}

func GetDiff(args []string) (*DiffResult, error) {
	diffArgs := append([]string{"diff"}, args...)
	diffOut, err := runGit(diffArgs...)
	if err != nil {
		return nil, err
	}

	statArgs := append([]string{"diff"}, args...)
	statArgs = append(statArgs, "--stat")
	statOut, err := runGitColored(statArgs...)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(diffOut) == "" {
		return nil, noChangesErr(args)
	}

	return &DiffResult{
		Stat:   statOut,
		Diff:   Truncate(diffOut, MaxDiffLength),
		RawLen: len([]rune(diffOut)),
	}, nil
}

// noChangesErr builds a helpful "no changes" error for an empty diff. Untracked
// files are invisible to git diff; surface them so the user can include them.
func noChangesErr(args []string) error {
	staged := false
	for _, a := range args {
		if a == "--staged" {
			staged = true
			break
		}
	}
	if staged {
		return errors.New("no changes to review (stage files with 'git add' first)")
	}
	untracked := GetUntrackedFiles()
	if len(untracked) == 0 {
		if n := stagedChangesCount(); n > 0 {
			return fmt.Errorf("no unstaged changes to review: %d file(s) are staged. Use 'mu git review --staged' to review them, or 'git reset' to unstage.", n)
		}
		return errors.New("no changes to review")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "no changes to review: %d untracked file(s) are not included by git diff", len(untracked))
	shown := 0
	for _, f := range untracked {
		if shown >= 3 {
			fmt.Fprintf(&sb, "\n  ... and %d more", len(untracked)-shown)
			break
		}
		fmt.Fprintf(&sb, "\n  - %s", f)
		shown++
	}
	sb.WriteString("\nInclude them with 'git add -N <file>' (intent-to-add) or 'git add' before reviewing.")
	return errors.New(sb.String())
}

func GetStagedDiff() (*DiffResult, error) {
	diffOut, err := runGit("diff", "--staged")
	if err != nil {
		return nil, err
	}

	statOut, err := runGitColored("diff", "--staged", "--stat")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(diffOut) == "" {
		return nil, fmt.Errorf("no changes to commit (use git add to stage files first)")
	}

	return &DiffResult{
		Stat:   statOut,
		Diff:   Truncate(diffOut, MaxDiffLength),
		RawLen: len([]rune(diffOut)),
	}, nil
}

func GetStagedNameStatus() (string, error) {
	return runGit("diff", "--staged", "--name-status")
}

func GetNameStatus(args []string) (string, error) {
	nsArgs := append([]string{"diff"}, args...)
	nsArgs = append(nsArgs, "--name-status")
	return runGit(nsArgs...)
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func runGitColored(args ...string) (string, error) {
	fullArgs := append([]string{"-c", "color.ui=always"}, args...)
	return runGit(fullArgs...)
}

func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "\n...(diff truncated)"
}

func RepoName() string {
	out, err := runGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "unknown"
	}
	return filepath.Base(strings.TrimSpace(out))
}

func CurrentBranch() string {
	out, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "HEAD"
	}
	return strings.TrimSpace(out)
}

func ShortCommit() string {
	out, err := runGit("rev-parse", "--short", "HEAD")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(out)
}

// ResolveRev resolves any git revision (hash, branch, tag, HEAD~n) to its full
// commit hash, or "" when it cannot be resolved.
func ResolveRev(rev string) string {
	out, err := runGit("rev-parse", rev+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// IsDirty reports whether the working tree has uncommitted changes
// (tracked modifications, staged changes, or untracked files).
func IsDirty() bool {
	out, err := runGit("status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

func PlainDiffStat(args []string) string {
	statArgs := append([]string{"diff"}, args...)
	statArgs = append(statArgs, "--stat")
	out, err := runGit(statArgs...)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func DirIsRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path
	return cmd.Run() == nil
}

func FileSafeName(s string) string {
	r := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		" ", "-",
		"#", "-",
		":", "-",
		"\"", "",
		"'", "",
	)
	return r.Replace(s)
}

func GetUntrackedFiles() []string {
	out, err := runGit("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// stagedChangesCount returns the number of files staged in the index.
func stagedChangesCount() int {
	out, err := runGit("diff", "--cached", "--name-only")
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}
