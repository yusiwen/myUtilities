package patch

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Options controls rendering.
type Options struct {
	// Color enables ANSI colours.
	Color bool
	// LineNumbers shows the old/new line-number gutter.
	LineNumbers bool
	// Theme selects the colour palette when Color is enabled.
	Theme Theme
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

// Render writes the parsed patch. Plain output (Color false) has no escape
// codes, so it can be piped or saved.
func Render(w io.Writer, files []File, opts Options) error {
	pal := paletteFor(opts.Theme)
	bw := bufio.NewWriter(w)
	for i := range files {
		if i > 0 {
			if _, err := fmt.Fprintln(bw); err != nil {
				return err
			}
		}
		for _, line := range Lines(files[i], opts) {
			if _, err := fmt.Fprintf(bw, "%s%s\n",
				paint(line.Gutter, pal.gutter, opts.Color),
				paint(line.Text, pal.color(line.Kind), opts.Color)); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

// gutterText sizes and pads the old/new line-number prefix.
func gutterText(oldNo, newNo, oldWidth, newWidth int) string {
	oldText, newText := "", ""
	if oldNo >= 0 {
		oldText = strconv.Itoa(oldNo)
	}
	if newNo >= 0 {
		newText = strconv.Itoa(newNo)
	}
	return fmt.Sprintf("%*s %*s | ", oldWidth, oldText, newWidth, newText)
}

func noNewlineLine(opts Options, oldWidth, newWidth int) string {
	indent := ""
	if opts.LineNumbers {
		indent = strings.Repeat(" ", oldWidth) + " " + strings.Repeat(" ", newWidth) + " | "
	}
	return indent + `\ No newline at end of file`
}

// lineWidths sizes the gutter from the highest line number any hunk can reach.
func lineWidths(f *File) (int, int) {
	oldWidth, newWidth := 1, 1
	for _, h := range f.Hunks {
		oldWidth = max(oldWidth, digits(h.OldStart+h.OldCount))
		newWidth = max(newWidth, digits(h.NewStart+h.NewCount))
	}
	return oldWidth, newWidth
}

func digits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		n /= 10
		d++
	}
	return d
}

func paint(s, code string, enabled bool) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}
