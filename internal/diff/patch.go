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
}

// Run renders the patch to stdout. A read or parse error exits 2.
func (o *PatchOptions) Run() error {
	color := false
	switch o.Color {
	case "always":
		color = true
	case "never":
		color = false
	default: // auto
		color = term.IsTerminal(int(os.Stdout.Fd()))
	}

	if err := o.render(os.Stdin, os.Stdout, color); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return nil
}

// render injects stdin and the output writer so the behaviour is testable.
func (o *PatchOptions) render(stdin io.Reader, w io.Writer, color bool) error {
	data, err := o.readInput(stdin)
	if err != nil {
		return err
	}

	files, err := patch.Parse(data)
	if err != nil {
		return fmt.Errorf("parse patch: %w", err)
	}
	if len(files) == 0 {
		if len(bytes.TrimSpace(data)) == 0 {
			return nil
		}
		return errors.New("input does not look like a unified diff")
	}

	return patch.Render(w, files, patch.Options{Color: color, LineNumbers: o.LineNumbers})
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
