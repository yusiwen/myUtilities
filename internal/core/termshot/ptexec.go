//go:build !windows

package termshot

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// PseudoTerminal defines the setup for a command to be run in a pseudo
// terminal, e.g. terminal size, or output settings.
type PseudoTerminal struct {
	name string
	args []string

	shell string

	cols   uint16
	rows   uint16
	resize bool

	stdout io.Writer
}

// NewPseudoTerminal creates a new pseudo terminal builder.
func NewPseudoTerminal() *PseudoTerminal {
	return &PseudoTerminal{
		shell:  "/bin/sh",
		resize: true,
		stdout: os.Stdout,
	}
}

// Cols sets the width/columns for the pseudo terminal.
func (c *PseudoTerminal) Cols(cols uint16) *PseudoTerminal {
	c.cols = cols
	return c
}

// Rows sets the lines/rows for the pseudo terminal.
func (c *PseudoTerminal) Rows(rows uint16) *PseudoTerminal {
	c.rows = rows
	return c
}

// Stdout sets the writer to be used for the standard output.
func (c *PseudoTerminal) Stdout(stdout io.Writer) *PseudoTerminal {
	c.stdout = stdout
	return c
}

// Command sets the command and arguments to be used.
func (c *PseudoTerminal) Command(name string, args ...string) *PseudoTerminal {
	c.name = name
	c.args = args
	return c
}

// Run runs the provided command/script with the given arguments in a pseudo
// terminal (PTY) so that the behavior is the same as if it were executed in a
// real terminal.
func (c *PseudoTerminal) Run() ([]byte, error) {
	if c.name == "" {
		return nil, fmt.Errorf("no command specified")
	}

	// Convenience hack in case the command contains a space, for example when
	// typical constructs like "foo | grep" are used.
	if strings.Contains(c.name, " ") {
		c.args = []string{
			"-c",
			strings.Join(append(
				[]string{c.name},
				c.args...,
			), " "),
		}
		c.name = c.shell
	}

	// Set RAW mode for Stdin
	if isTerminal(os.Stdin) {
		oldState, rawErr := term.MakeRaw(int(os.Stdin.Fd()))
		if rawErr != nil {
			return nil, fmt.Errorf("failed to enable RAW mode for Stdin: %w", rawErr)
		}

		// And make sure to restore the original mode eventually
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
	}

	// collect all errors along the way
	var errors = []error{}

	pt, err := c.pseudoTerminal(exec.Command(c.name, c.args...)) // #nosec G204
	if err != nil {
		return nil, err
	}

	// Support terminal resizing
	if c.resize && isTerminal(os.Stdin) {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		go func() {
			for range ch {
				if ptyErr := pty.InheritSize(os.Stdin, pt); ptyErr != nil {
					errors = append(errors, fmt.Errorf("error resizing PTY: %w", ptyErr))
				}
			}
		}()

		ch <- syscall.SIGWINCH
		defer func() {
			signal.Stop(ch)
			close(ch)
		}()
	}

	go func() {
		defer func() { _ = pt.Close() }()
		_, copyErr := io.Copy(pt, os.Stdin)
		if copyErr != nil {
			errors = append(errors, copyErr)
		}
	}()

	var buf bytes.Buffer
	if err = copyBuf(io.MultiWriter(c.stdout, &buf), pt); err != nil {
		return nil, err
	}

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "issues in background tasks:\n")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "- %v\n", err.Error())
		}
	}

	return buf.Bytes(), nil
}

func (c *PseudoTerminal) pseudoTerminal(cmd *exec.Cmd) (*os.File, error) {
	if c.cols == 0 && c.rows == 0 {
		return pty.Start(cmd)
	}

	size, err := pty.GetsizeFull(os.Stdout)
	if err != nil {
		// Obtaining the terminal size is prone to error in CI systems, so only
		// fail if CI is not set.
		if !isCI() {
			return nil, fmt.Errorf("failed to get size: %w", err)
		}

		// For CI systems, assume a reasonable default even if the terminal
		// size cannot be obtained through ioctl.
		size = &pty.Winsize{Rows: 25, Cols: 80}
	}

	// Overwrite rows if a fixed value is configured.
	if c.rows != 0 {
		size.Rows = c.rows
	}

	// Overwrite columns if a fixed value is configured.
	if c.cols != 0 {
		size.Cols = c.cols
	}

	// With fixed rows/cols, terminal resizing support is not useful.
	c.resize = false

	return pty.StartWithSize(cmd, size)
}

func copyBuf(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if err != nil {
		switch terr := err.(type) { //nolint:gocritic
		case *os.PathError:
			// Workaround for https://github.com/creack/pty/issues/100 where on
			// Linux the pseudo terminal process can finish while the tool is
			// still reading. Assuming the content is already read, this error
			// is treated the same as an EOF.
			if terr.Op == "read" && terr.Path == "/dev/ptmx" {
				return nil
			}
		}
	}

	return err
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) ||
		isatty.IsCygwinTerminal(f.Fd())
}

func isCI() bool {
	ci, ok := os.LookupEnv("CI")
	return ok && ci == "true"
}
