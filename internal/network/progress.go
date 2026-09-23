package network

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/morikuni/aec"
	"github.com/yusiwen/myUtilities/internal/core/downloader"
	"golang.org/x/term"
)

// progressRenderer draws the live transfer display:
//
//	file.iso  45.2%  ████████████░░░░░░░░  452.1 MiB/1000.0 MiB  24.3 MiB/s  ETA 00:22  conn 6/8
//
// With verbose enabled it adds one line per connection. On a non-TTY it falls
// back to at most one plain line per second, and --quiet disables it entirely.
//
// It is used from several goroutines: the downloader reports progress from its
// reporter goroutine while workers (retries) and the resume-state flusher call
// Warn/Log from their own. Every method therefore holds mu.
type progressRenderer struct {
	mu       sync.Mutex
	out      io.Writer
	isTTY    bool
	color    bool
	verbose  bool
	quiet    bool
	width    int
	rendered int
	lastLog  time.Time
	closed   bool
}

func newProgressRenderer(out *os.File, verbose, quiet bool) *progressRenderer {
	isTTY := term.IsTerminal(int(out.Fd()))
	return &progressRenderer{
		out:     out,
		isTTY:   isTTY,
		color:   isTTY && os.Getenv("NO_COLOR") == "",
		verbose: verbose,
		quiet:   quiet,
		width:   terminalWidth(out),
	}
}

func terminalWidth(out *os.File) int {
	if width, _, err := term.GetSize(int(out.Fd())); err == nil && width > 0 {
		return width
	}
	return 100
}

// Update implements downloader.Progress.
func (r *progressRenderer) Update(s downloader.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.quiet || r.closed {
		return
	}
	if r.isTTY {
		r.width = terminalWidth(os.Stderr)
	} else {
		now := time.Now()
		if now.Sub(r.lastLog) < time.Second {
			return
		}
		r.lastLog = now
	}

	lines := []string{r.aggregateLine(s)}
	if r.verbose && s.Workers > 1 {
		for _, conn := range s.Connections {
			lines = append(lines, r.connLine(conn, s.Total, s.SizeKnown))
		}
	}

	if !r.isTTY {
		fmt.Fprintln(r.out, lines[0])
		return
	}
	r.render(lines)
}

// Log prints an informational line above the live region without corrupting it.
// It may be called from any goroutine.
func (r *progressRenderer) Log(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.quiet {
		return
	}
	r.printAbove(fmt.Sprintf(format, args...))
}

// Warn prints a warning line above the live region (also when --quiet). It may
// be called from any goroutine.
func (r *progressRenderer) Warn(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.printAbove(r.paint("warning: ", aec.YellowF) + fmt.Sprintf(format, args...))
}

func (r *progressRenderer) printAbove(msg string) {
	if r.closed {
		return
	}
	r.clearRegion()
	fmt.Fprintln(r.out, msg)
}

// Close clears the live region and restores the cursor.
func (r *progressRenderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	if r.isTTY && r.rendered > 0 {
		r.clearRegion()
		fmt.Fprint(r.out, "\x1b[?25h")
	}
}

// clearRegion moves to the top of the region and blanks every line.
func (r *progressRenderer) clearRegion() {
	if !r.isTTY || r.rendered == 0 {
		return
	}
	fmt.Fprintf(r.out, "\x1b[%dA", r.rendered)
	for i := 0; i < r.rendered; i++ {
		fmt.Fprint(r.out, "\r\x1b[K\n")
	}
	r.rendered = 0
}

// render repaints all region lines in place.
func (r *progressRenderer) render(lines []string) {
	if r.rendered == 0 {
		fmt.Fprint(r.out, "\x1b[?25l") // hide the cursor while repainting
	} else {
		fmt.Fprintf(r.out, "\x1b[%dA", r.rendered)
	}
	for _, line := range lines {
		fmt.Fprintf(r.out, "\r\x1b[K%s\n", line)
	}
	if r.rendered > len(lines) {
		for i := len(lines); i < r.rendered; i++ {
			fmt.Fprint(r.out, "\r\x1b[K\n")
		}
	}
	if len(lines) > r.rendered {
		r.rendered = len(lines)
	}
}

// aggregateLine formats the overall transfer status.
func (r *progressRenderer) aggregateLine(s downloader.Snapshot) string {
	name := s.Output
	if r.color {
		name = aec.Apply(name, aec.Bold)
	}

	parts := make([]string, 0, 5)
	if s.SizeKnown && s.Total > 0 {
		ratio := float64(s.Done) / float64(s.Total)
		if ratio > 1 {
			ratio = 1
		}
		pct := fmt.Sprintf("%5.1f%%", ratio*100)
		if r.color {
			pct = aec.Apply(pct, aec.CyanF)
		}
		parts = append(parts, pct, r.bar(ratio, r.barWidth(s, name)))
		parts = append(parts, fmt.Sprintf("%s/%s", downloader.FormatSize(s.Done), downloader.FormatSize(s.Total)))
	} else {
		parts = append(parts, downloader.FormatSize(s.Done))
	}

	speed := formatSpeed(s.Speed)
	if r.color {
		speed = aec.Apply(speed, aec.YellowF)
	}
	parts = append(parts, speed)

	if s.ETA > 0 {
		parts = append(parts, "ETA "+formatDuration(s.ETA))
	}
	parts = append(parts, fmt.Sprintf("conn %d/%d", s.ActiveWorkers, s.Workers))

	return name + "  " + strings.Join(parts, "  ")
}

// connLine formats one connection's contribution.
func (r *progressRenderer) connLine(c downloader.ConnStat, total int64, sizeKnown bool) string {
	marker := "·"
	if c.Active {
		marker = "▸"
	}
	ratio := 0.0
	if sizeKnown && total > 0 {
		ratio = float64(c.Bytes) / float64(total)
		if ratio > 1 {
			ratio = 1
		}
	}
	line := fmt.Sprintf("  [%d] %s %10s  %9s  %s",
		c.ID, marker, downloader.FormatSize(c.Bytes), formatSpeed(c.Speed), r.bar(ratio, 12))
	if r.color && !c.Active {
		line = aec.Apply(line, aec.Faint)
	}
	return line
}

// barWidth keeps the aggregate line inside the terminal.
func (r *progressRenderer) barWidth(s downloader.Snapshot, name string) int {
	width := r.width
	if width <= 0 {
		width = 100
	}
	// Everything except the bar: name, percentage, sizes, speed, ETA, conns.
	reserved := len(name) + 68
	size := width - reserved
	if size < 10 {
		size = 10
	}
	if size > 40 {
		size = 40
	}
	return size
}

// bar renders a progress bar of the given width.
func (r *progressRenderer) bar(ratio float64, width int) string {
	if width < 2 {
		width = 2
	}
	filled := int(ratio*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	full := strings.Repeat("█", filled)
	empty := strings.Repeat("░", width-filled)
	if !r.color {
		return full + empty
	}
	return aec.Apply(full, aec.GreenF) + aec.Apply(empty, aec.Faint)
}

func (r *progressRenderer) paint(s string, styles ...aec.ANSI) string {
	if !r.color {
		return s
	}
	return aec.Apply(s, styles...)
}

// formatSpeed renders a byte-per-second rate.
func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond <= 0 {
		return "-- /s"
	}
	return downloader.FormatSize(int64(bytesPerSecond)) + "/s"
}

// formatDuration renders an ETA compactly (mm:ss or h:mm:ss).
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds() + 0.5)
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
