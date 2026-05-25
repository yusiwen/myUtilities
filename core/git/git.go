package git

import (
	"fmt"
	"os/exec"
	"strings"
)

const MaxDiffLength = 8000

type DiffResult struct {
	Stat string
	Diff string
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

func GetStagedDiff() (*DiffResult, error) {
	diffOut, err := runGit("diff", "--staged")
	if err != nil {
		return nil, err
	}

	statOut, err := runGit("diff", "--staged", "--stat")
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(diffOut) == "" {
		return nil, fmt.Errorf("no changes to commit (use git add to stage files first)")
	}

	return &DiffResult{
		Stat: statOut,
		Diff: truncate(diffOut, MaxDiffLength),
	}, nil
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

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "\n...(diff truncated)"
}
