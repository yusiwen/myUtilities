//go:build windows

package termshot

import (
	"fmt"
	"io"
)

// PseudoTerminal is a no-op on Windows, where pseudo terminals are not provided
// by creack/pty. The methods below keep the API surface identical to the Unix
// implementation so the `mu termshot` command still compiles on Windows; calls
// fail at runtime instead.
type PseudoTerminal struct{}

// NewPseudoTerminal creates a pseudo terminal builder (no-op on Windows).
func NewPseudoTerminal() *PseudoTerminal { return &PseudoTerminal{} }

// Cols is a no-op on Windows.
func (c *PseudoTerminal) Cols(cols uint16) *PseudoTerminal { return c }

// Rows is a no-op on Windows.
func (c *PseudoTerminal) Rows(rows uint16) *PseudoTerminal { return c }

// Stdout is a no-op on Windows.
func (c *PseudoTerminal) Stdout(w io.Writer) *PseudoTerminal { return c }

// Command is a no-op on Windows.
func (c *PseudoTerminal) Command(name string, args ...string) *PseudoTerminal { return c }

// Run is not supported on Windows.
func (c *PseudoTerminal) Run() ([]byte, error) {
	return nil, fmt.Errorf("pseudo terminals are not supported on this platform")
}
