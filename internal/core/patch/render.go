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
	bw := bufio.NewWriter(w)
	for i := range files {
		if i > 0 {
			if _, err := fmt.Fprintln(bw); err != nil {
				return err
			}
		}
		renderFile(bw, &files[i], opts)
	}
	return bw.Flush()
}

func renderFile(w io.Writer, f *File, opts Options) {
	for _, line := range f.Header {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			fmt.Fprintln(w, paint(line, ansiBold, opts.Color))
		case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
			fmt.Fprintln(w, paint(line, ansiYellow, opts.Color))
		default:
			fmt.Fprintln(w, paint(line, ansiDim, opts.Color))
		}
	}
	if f.OldLine != "" {
		fmt.Fprintln(w, paint(f.OldLine, ansiRed, opts.Color))
	}
	if f.NewLine != "" {
		fmt.Fprintln(w, paint(f.NewLine, ansiGreen, opts.Color))
	}

	oldWidth, newWidth := lineWidths(f)
	for _, h := range f.Hunks {
		fmt.Fprintln(w, paint(h.Header, ansiBold+ansiCyan, opts.Color))
		oldNo, newNo := h.OldStart, h.NewStart
		for _, line := range h.Lines {
			switch line.Kind {
			case Context:
				writeLine(w, opts, oldWidth, newWidth, oldNo, newNo, ' ', line.Text, ansiDim)
				oldNo++
				newNo++
			case Delete:
				writeLine(w, opts, oldWidth, newWidth, oldNo, -1, '-', line.Text, ansiRed)
				oldNo++
			case Add:
				writeLine(w, opts, oldWidth, newWidth, -1, newNo, '+', line.Text, ansiGreen)
				newNo++
			case NoNewline:
				fmt.Fprintln(w, paint(noNewlineLine(opts, oldWidth, newWidth), ansiDim, opts.Color))
			}
		}
	}
}

func writeLine(w io.Writer, opts Options, oldWidth, newWidth, oldNo, newNo int, marker byte, text, color string) {
	var b strings.Builder
	if opts.LineNumbers {
		b.WriteString(gutter(oldNo, newNo, oldWidth, newWidth, opts.Color))
	}
	if opts.Color {
		b.WriteString(color)
	}
	b.WriteByte(marker)
	b.WriteString(text)
	if opts.Color {
		b.WriteString(ansiReset)
	}
	b.WriteByte('\n')
	_, _ = io.WriteString(w, b.String())
}

func gutter(oldNo, newNo, oldWidth, newWidth int, color bool) string {
	oldText, newText := "", ""
	if oldNo >= 0 {
		oldText = strconv.Itoa(oldNo)
	}
	if newNo >= 0 {
		newText = strconv.Itoa(newNo)
	}
	return paint(fmt.Sprintf("%*s %*s | ", oldWidth, oldText, newWidth, newText), ansiDim, color)
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
