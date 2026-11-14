package merge

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeMatchesGit compares the engine with git merge-file on generated
// documents. Clean merges must be byte-identical; conflicting ones must at least
// agree that there is a conflict. Base lines are unique so both diff
// implementations see the same unambiguous edits.
func TestMergeMatchesGit(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed; skipping differential test")
	}

	rng := rand.New(rand.NewSource(20261114))
	conflictsSeen := 0
	for i := 0; i < 200; i++ {
		base := randomDocument(rng, i)
		ours := randomEdit(rng, base)
		theirs := randomEdit(rng, base)
		if checkAgainstGit(t, gitBin, base, ours, theirs) {
			conflictsSeen++
		}
	}
	// Guard against a generator that only produces clean merges, which would make
	// the conflict half of the comparison vacuous.
	if conflictsSeen < 20 {
		t.Fatalf("only %d of 200 generated cases conflicted; the generator no longer exercises conflicts", conflictsSeen)
	}
	t.Logf("%d/200 generated cases conflicted; the rest matched git byte for byte", conflictsSeen)

	// Both sides making the same edit must merge cleanly to that edit.
	for i := 0; i < 30; i++ {
		base := randomDocument(rng, 1000+i)
		edited := randomEdit(rng, base)
		if checkAgainstGit(t, gitBin, base, edited, edited) {
			t.Fatalf("identical edits on both sides produced a conflict\nbase=%q\nedited=%q", base, edited)
		}
	}
}

func checkAgainstGit(t *testing.T, gitBin string, base, ours, theirs []byte) bool {
	t.Helper()

	res, err := Merge(base, ours, theirs, Options{
		Style:  StyleDiff3,
		Labels: Labels{Ours: "ours", Base: "base", Theirs: "theirs"},
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	want, gitConflicts, err := gitMergeFile(gitBin, base, ours, theirs)
	if err != nil {
		t.Fatalf("git merge-file: %v", err)
	}

	if gitConflicts {
		if res.Conflicts == 0 {
			t.Fatalf("git reports a conflict, engine reports none\nbase=%q\nours=%q\ntheirs=%q", base, ours, theirs)
		}
		return true
	}
	if res.Conflicts != 0 {
		t.Fatalf("git merges cleanly, engine reports %d conflict(s)\nbase=%q\nours=%q\ntheirs=%q\ngot=%q",
			res.Conflicts, base, ours, theirs, res.Text)
	}
	if !bytes.Equal(res.Text, want) {
		t.Fatalf("clean merge differs from git\nbase=%q\nours=%q\ntheirs=%q\ngot=%q\nwant=%q",
			base, ours, theirs, res.Text, want)
	}
	return false
}

// gitMergeFile runs git merge-file and reports whether it saw conflicts.
func gitMergeFile(gitBin string, base, ours, theirs []byte) ([]byte, bool, error) {
	dir, err := os.MkdirTemp("", "mu-merge-git-")
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	for _, f := range []struct {
		name string
		data []byte
	}{
		{"base.txt", base},
		{"ours.txt", ours},
		{"theirs.txt", theirs},
	} {
		if err := os.WriteFile(filepath.Join(dir, f.name), f.data, 0o600); err != nil {
			return nil, false, err
		}
	}

	cmd := exec.Command(gitBin, "merge-file", "-p", "--diff3",
		"-L", "ours", "-L", "base", "-L", "theirs",
		"ours.txt", "base.txt", "theirs.txt")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, false, err
		}
		// git merge-file exits with the number of conflicts (1-127); 255 is an
		// error (negative status wrapped by the shell).
		if code := exitErr.ExitCode(); code > 0 && code < 255 {
			return stdout.Bytes(), true, nil
		}
		return nil, false, fmt.Errorf("git merge-file failed (%d): %s", exitErr.ExitCode(), stderr.String())
	}
	return stdout.Bytes(), false, nil
}

// randomDocument builds a document whose lines are unique, so both diffs align
// unambiguously and the test measures merge semantics rather than diff heuristics.
func randomDocument(rng *rand.Rand, docID int) []byte {
	n := 1 + rng.Intn(10)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "base-%d-%d\n", docID, i)
	}
	return []byte(sb.String())
}

func randomEdit(rng *rand.Rand, doc []byte) []byte {
	lines := splitLines(doc)
	for i := 0; i < rng.Intn(4); i++ {
		switch rng.Intn(3) {
		case 0: // insert
			pos := rng.Intn(len(lines) + 1)
			lines = spliceLines(lines, pos, pos, randomEditLines(rng))
		case 1: // delete
			if len(lines) == 0 {
				continue
			}
			start := rng.Intn(len(lines))
			end := start + 1 + rng.Intn(min(2, len(lines)-start))
			lines = spliceLines(lines, start, end, nil)
		default: // replace
			if len(lines) == 0 {
				continue
			}
			start := rng.Intn(len(lines))
			end := start + 1 + rng.Intn(min(2, len(lines)-start))
			lines = spliceLines(lines, start, end, randomEditLines(rng))
		}
	}
	return []byte(strings.Join(lines, ""))
}

func randomEditLines(rng *rand.Rand) []string {
	n := 1 + rng.Intn(3)
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("edit-%d-%d\n", rng.Intn(1<<20), i)
	}
	return out
}

func spliceLines(lines []string, start, end int, repl []string) []string {
	out := make([]string, 0, len(lines)-(end-start)+len(repl))
	out = append(out, lines[:start]...)
	out = append(out, repl...)
	out = append(out, lines[end:]...)
	return out
}
