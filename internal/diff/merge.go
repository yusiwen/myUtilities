package diff

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yusiwen/myUtilities/internal/core/merge"
)

// maxMergeInputBytes caps one merge input. The engine holds all three files in
// memory and works line by line, so a larger input would only be slow; callers
// can split it if they really need to.
const maxMergeInputBytes = 10 << 20

// MergeOptions is `mu diff merge`: a generic, git-agnostic three-way merge.
type MergeOptions struct {
	Base   string `name:"base" required:"" help:"Common ancestor file, '-' reads stdin."`
	Ours   string `name:"ours" required:"" help:"Our version, '-' reads stdin."`
	Theirs string `name:"theirs" required:"" help:"Their version, '-' reads stdin."`

	Style       string `help:"Conflict marker style." enum:"merge,diff3" default:"merge"`
	OursLabel   string `help:"Label in the ours marker (default: file name)."`
	BaseLabel   string `help:"Label in the base marker (default: file name)."`
	TheirsLabel string `help:"Label in the theirs marker (default: file name)."`
	MarkerSize  int    `help:"Number of characters in each conflict marker." default:"7"`
	Check       bool   `help:"Only report whether the merge has conflicts; write nothing."`
}

// Run writes the merged document to stdout. Exit codes follow git merge-file:
// 0 clean, 1 conflicts, 2 read or usage error.
func (o *MergeOptions) Run() error {
	res, err := o.mergeWith(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if !o.Check {
		if _, err := os.Stdout.Write(res.Text); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v\n", err)
			os.Exit(2)
		}
	}
	if res.Conflicts > 0 {
		fmt.Fprintf(os.Stderr, "merge: %d conflict(s)\n", res.Conflicts)
		os.Exit(1)
	}
	return nil
}

// mergeWith reads the three inputs and merges them. stdin is injected so the
// behaviour is testable without replacing os.Stdin.
func (o *MergeOptions) mergeWith(stdin io.Reader) (merge.Result, error) {
	style, err := conflictStyle(o.Style)
	if err != nil {
		return merge.Result{}, err
	}

	stdinUsed := false
	read := func(role, path string) ([]byte, error) {
		name := path
		var data []byte
		var err error
		if path == "-" {
			if stdinUsed {
				return nil, fmt.Errorf("only one of --base/--ours/--theirs may read stdin")
			}
			stdinUsed = true
			name = "stdin"
			data, err = io.ReadAll(io.LimitReader(stdin, maxMergeInputBytes+1))
			if err != nil {
				return nil, fmt.Errorf("read %s from stdin: %w", role, err)
			}
		} else {
			data, err = os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
		}
		if len(data) > maxMergeInputBytes {
			return nil, fmt.Errorf("%s is larger than the %d MiB merge limit", name, maxMergeInputBytes>>20)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil, fmt.Errorf("%s looks like a binary file, not text", name)
		}
		return data, nil
	}

	base, err := read("base", o.Base)
	if err != nil {
		return merge.Result{}, err
	}
	ours, err := read("ours", o.Ours)
	if err != nil {
		return merge.Result{}, err
	}
	theirs, err := read("theirs", o.Theirs)
	if err != nil {
		return merge.Result{}, err
	}

	labels := merge.Labels{
		Ours:   mergeLabel(o.OursLabel, "ours", o.Ours),
		Base:   mergeLabel(o.BaseLabel, "base", o.Base),
		Theirs: mergeLabel(o.TheirsLabel, "theirs", o.Theirs),
	}
	return merge.Merge(base, ours, theirs, merge.Options{
		Style:      style,
		Labels:     labels,
		MarkerSize: o.MarkerSize,
	})
}

// mergeLabel picks the explicit label, or the role name for stdin, or the file's
// base name.
func mergeLabel(explicit, role, path string) string {
	if explicit != "" {
		return explicit
	}
	if path == "-" {
		return role
	}
	return filepath.Base(path)
}

func conflictStyle(name string) (merge.Style, error) {
	switch name {
	case "", "merge":
		return merge.StyleMerge, nil
	case "diff3":
		return merge.StyleDiff3, nil
	default:
		return 0, fmt.Errorf("unknown conflict style %q", name)
	}
}
