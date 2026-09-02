package termshot

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	coretermshot "github.com/yusiwen/myUtilities/internal/core/termshot"
)

// Options is the `mu termshot` command. It runs a command in a pseudo terminal,
// captures the (ANSI rich) output and renders a screenshot that resembles a
// terminal window.
//
// Like time, watch or perf, prefix any command with `mu termshot`:
//
//	mu termshot ls -a
//	mu termshot -- "ls -1 | grep go"
//
// Use `--` (or quote the whole command) so flags before it belong to termshot
// and everything after belongs to the target command.
type Options struct {
	// Command is the target command and its arguments. Everything after `--`
	// (or the whole quoted string) is captured here.
	Command []string `arg:"" optional:"" name:"command" help:"Command to run (use '--' before the command)."`

	Edit       bool   `short:"e" help:"Edit the output before creating the screenshot."`
	ShowCmd    bool   `short:"c" help:"Include the command in the screenshot."`
	Columns    int    `short:"C" default:"0" help:"Force a fixed number of columns in the screenshot."`
	Margin     int    `short:"m" default:"48" help:"Set the margin around the window."`
	Padding    int    `short:"p" default:"24" help:"Set the padding around the content inside the window."`
	NoDecorate bool   `name:"no-decoration" help:"Do not draw window decorations."`
	NoShadow   bool   `name:"no-shadow" help:"Do not draw the window shadow."`
	ClipCanvas bool   `short:"s" name:"clip-canvas" help:"Clip the canvas to the visible image area (no margin)."`
	Filename   string `short:"f" default:"out.png" help:"Filename of the screenshot."`
	Clipboard  bool   `short:"b" help:"Copy the screenshot to the OS clipboard instead of saving a file."`
	RawWrite   string `name:"raw-write" help:"Write raw output to a file (- for stdout) instead of rendering a screenshot."`
	RawRead    string `name:"raw-read" help:"Read raw input from a file (- for stdin) instead of running a command."`
}

// Run executes the command and renders the screenshot.
func (o *Options) Run() error {
	if len(o.Command) == 0 && o.RawRead == "" {
		return fmt.Errorf("no command specified; use 'mu termshot -- <command>' (or --raw-read <file>)")
	}

	scaffold := coretermshot.NewImageCreator()
	pt := coretermshot.NewPseudoTerminal()

	if o.Columns > 0 {
		scaffold.SetColumns(o.Columns)
		pt.Cols(uint16(o.Columns))
	}

	if o.Margin < 0 {
		return fmt.Errorf("margin must be zero or greater: not %d", o.Margin)
	}
	scaffold.SetMargin(float64(o.Margin))

	if o.Padding < 0 {
		return fmt.Errorf("padding must be zero or greater: not %d", o.Padding)
	}
	scaffold.SetPadding(float64(o.Padding))

	scaffold.DrawShadow(!o.NoShadow)
	scaffold.DrawDecorations(!o.NoDecorate)
	scaffold.ClipCanvas(o.ClipCanvas)

	// Optional: prepend command line arguments to the output content.
	if o.ShowCmd && o.RawRead == "" {
		if err := scaffold.AddCommand(o.Command...); err != nil {
			return err
		}
	}

	// Get the actual content for the screenshot.
	var buf bytes.Buffer
	if o.RawRead == "" {
		// Run the command in a pseudo terminal and capture its output.
		out, err := pt.Command(o.Command[0], o.Command[1:]...).Run()
		if err != nil {
			return fmt.Errorf("failed to run command in pseudo terminal: %w", err)
		}
		buf.Write(out)
	} else {
		// Read the content from an existing file instead of running a command.
		data, err := readInput(o.RawRead)
		if err != nil {
			return fmt.Errorf("failed to read contents: %w", err)
		}
		buf.Write(data)
	}

	// Allow manual override of the command output content.
	if o.Edit && o.RawRead == "" {
		edited, err := editContent(buf.Bytes())
		if err != nil {
			return err
		}
		buf.Reset()
		buf.Write(edited)
	}

	// Add the captured output to the scaffold.
	if err := scaffold.AddContent(&buf); err != nil {
		return err
	}

	// Optional: save content as-is to a file.
	if o.RawWrite != "" {
		var output *os.File
		var err error
		if o.RawWrite == "-" {
			output = os.Stdout
		} else {
			output, err = os.Create(filepath.Clean(o.RawWrite))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer func() { _ = output.Close() }()
		}
		return scaffold.WriteRaw(output)
	}

	// Optional: save image to clipboard.
	if o.Clipboard {
		return coretermshot.SaveToClipboard(scaffold)
	}

	// Save image to file.
	filename := o.Filename
	if filename == "" {
		filename = "out.png"
	}
	if ext := filepath.Ext(filename); ext != ".png" {
		return fmt.Errorf("file extension %q of filename %q is not supported, only png is supported", ext, filename)
	}

	file, err := os.Create(filepath.Clean(filename))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = file.Close() }()
	return scaffold.WritePNG(file)
}

func readInput(name string) ([]byte, error) {
	switch name {
	case "-":
		return io.ReadAll(os.Stdin)
	default:
		return os.ReadFile(filepath.Clean(name))
	}
}

// editContent opens the content in $EDITOR (falling back to vi) and returns the
// edited result, so users can strip unwanted or sensitive output.
func editContent(content []byte) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "mu-termshot-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if err := os.WriteFile(tmpFile.Name(), content, 0o644); err != nil {
		return nil, err
	}

	editor := os.Getenv("EDITOR")
	if len(editor) == 0 {
		editor = "vi"
	}
	if _, err := coretermshot.NewPseudoTerminal().Command(editor, tmpFile.Name()).Run(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	return data, nil
}
