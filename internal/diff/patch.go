package diff

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/yusiwen/myUtilities/internal/core/patch"
	"golang.org/x/term"
)

// maxPatchInputBytes caps the rendered patch. A whole diff is held in memory to
// parse it, and anything larger is better handled by the producer.
const maxPatchInputBytes = 50 << 20

// PatchOptions is `mu diff patch`: render a unified diff produced elsewhere.
type PatchOptions struct {
	File        string `arg:"" optional:"" name:"file" help:"Unified diff file to render (default: stdin)."`
	Color       string `help:"Color output: auto, always, never." enum:"auto,always,never" default:"auto"`
	LineNumbers bool   `negatable:"" default:"true" help:"Show old and new line numbers."`
	TUI         bool   `help:"Open an interactive file list and diff viewer (requires a terminal)."`
	Theme       string `help:"Color theme." enum:"default,nord" default:"nord"`
}

// Run renders the patch to stdout, or opens the interactive viewer with --tui.
// A read or parse error exits 2.
func (o *PatchOptions) Run() error {
	if err := o.run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return nil
}

// run is Run without the process exit, so the bare `mu diff` command can share
// the patch path and tests can drive it.
func (o *PatchOptions) run(stdin io.Reader, stdout *os.File) error {
	files, err := o.parseInput(stdin)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	return renderFiles(files, o.renderConfig(), stdin, stdout)
}

func (o *PatchOptions) renderConfig() renderConfig {
	return renderConfig{
		color:       o.Color,
		lineNumbers: o.LineNumbers,
		tui:         o.TUI,
		theme:       patchTheme(o.Theme),
		title:       "mu diff patch",
	}
}

// renderConfig is the presentation half shared by `mu diff patch` and the bare
// `mu diff` command.
type renderConfig struct {
	color       string
	lineNumbers bool
	tui         bool
	theme       patch.Theme
	title       string
}

// renderFiles writes the parsed files, or opens the interactive viewer.
func renderFiles(files []patch.File, cfg renderConfig, stdin io.Reader, stdout *os.File) error {
	opts := patch.Options{LineNumbers: cfg.lineNumbers, Theme: cfg.theme}
	if cfg.tui {
		return runTUI(files, opts, cfg.title, stdin, stdout)
	}
	opts.Color = colorEnabled(cfg.color, stdout)
	return patch.Render(stdout, files, opts)
}

// colorEnabled resolves --color against the output terminal.
func colorEnabled(mode string, out *os.File) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // auto
		return term.IsTerminal(int(out.Fd()))
	}
}

// render injects stdin and the output writer so the plain path is testable.
func (o *PatchOptions) render(stdin io.Reader, w io.Writer, color bool) error {
	files, err := o.parseInput(stdin)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	return patch.Render(w, files, patch.Options{Color: color, LineNumbers: o.LineNumbers, Theme: patchTheme(o.Theme)})
}

// parseInput reads and parses the patch. Empty input yields no files and no
// error; text that is not a diff is rejected.
func (o *PatchOptions) parseInput(stdin io.Reader) ([]patch.File, error) {
	data, err := o.readInput(stdin)
	if err != nil {
		return nil, err
	}
	files, err := patch.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}
	if len(files) == 0 {
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, nil
		}
		return nil, errors.New("input does not look like a unified diff")
	}
	return files, nil
}

func (o *PatchOptions) readInput(stdin io.Reader) ([]byte, error) {
	name := o.File
	var data []byte
	var err error

	if o.File == "" || o.File == "-" {
		name = "stdin"
		data, err = io.ReadAll(io.LimitReader(stdin, maxPatchInputBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(o.File)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", o.File, err)
		}
	}

	if len(data) > maxPatchInputBytes {
		return nil, fmt.Errorf("%s is larger than the %d MiB patch limit", name, maxPatchInputBytes>>20)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("%s looks like a binary file, not a unified diff", name)
	}
	return data, nil
}
