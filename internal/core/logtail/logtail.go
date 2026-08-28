// Package logtail provides log file tailing and filtering utilities.
package logtail

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	LevelUnknown Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the canonical lowercase name of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// ParseLevel parses a log level string (case-insensitive) into a Level.
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal", "critical":
		return LevelFatal
	default:
		return LevelUnknown
	}
}

// Line is a single parsed log entry.
type Line struct {
	Raw     string    // raw line text
	Level   Level     // detected level
	Time    time.Time // parsed timestamp (zero if none found)
	HasTime bool      // whether a timestamp was parsed
	IsJSON  bool      // whether the line is a JSON object
}

// levelRe matches a log-level keyword anywhere in the line as a whole word.
// This covers common formats like "INFO msg", "[ERROR] msg", "ts=... LEVEL msg".
var levelRe = regexp.MustCompile(`(?i)\b(debug|info|warn|warning|error|fatal|critical)\b`)

// timeRe is a loose timestamp pattern used as a fallback when JSON parsing
// does not yield a timestamp. It matches ISO-8601-ish and common log prefixes.
var timeRe = regexp.MustCompile(`^\[?(\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\]?(?:\s|$)`)

// ParseLine inspects a raw log line and extracts level, timestamp, and JSON
// detection. It handles both plain-text and JSON-line formats.
func ParseLine(raw string) Line {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Line{Raw: raw}
	}

	line := Line{Raw: raw}

	// Try JSON first.
	if trimmed[0] == '{' || trimmed[0] == '[' {
		var m map[string]any
		if err := json.Unmarshal([]byte(trimmed), &m); err == nil {
			line.IsJSON = true
			line.Level = levelFromJSON(m)
			line.Time, line.HasTime = timeFromJSON(m)
			return line
		}
	}

	// Plain-text level detection.
	if m := levelRe.FindStringSubmatch(trimmed); m != nil {
		line.Level = ParseLevel(m[1])
	}

	// Plain-text timestamp detection.
	if m := timeRe.FindStringSubmatch(trimmed); m != nil {
		line.Time, line.HasTime = parseTimeString(m[1])
	}

	return line
}

// levelFromJSON extracts a log level from a JSON object. It checks several
// common field names.
func levelFromJSON(m map[string]any) Level {
	for _, key := range []string{"level", "severity", "lvl", "loglevel", "log_level"} {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case string:
				return ParseLevel(t)
			case float64:
				switch {
				case t >= 5:
					return LevelFatal
				case t >= 4:
					return LevelError
				case t >= 3:
					return LevelWarn
				case t >= 2:
					return LevelInfo
				default:
					return LevelDebug
				}
			}
		}
	}
	return LevelUnknown
}

// timeFromJSON extracts a timestamp from a JSON object.
func timeFromJSON(m map[string]any) (time.Time, bool) {
	for _, key := range []string{"time", "timestamp", "ts", "@timestamp", "datetime"} {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if tm, err := time.Parse(time.RFC3339Nano, t); err == nil {
				return tm, true
			}
			if tm, err := time.Parse("2006-01-02 15:04:05.999999999", t); err == nil {
				return tm, true
			}
			if tm, err := time.Parse("2006-01-02 15:04:05", t); err == nil {
				return tm, true
			}
		case float64:
			// Unix timestamp (seconds or milliseconds).
			if t > 1e12 {
				return time.UnixMilli(int64(t)), true
			}
			return time.Unix(int64(t), 0), true
		}
	}
	return time.Time{}, false
}

// parseTimeString tries several layouts for a plain-text timestamp.
func parseTimeString(s string) (time.Time, bool) {
	layouts := []string{
		"2006-01-02T15:04:05.999999999Z07:00", // RFC3339Nano
		"2006-01-02T15:04:05Z07:00",           // RFC3339
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"01/02/2006 15:04:05",
	}
	for _, layout := range layouts {
		if tm, err := time.Parse(layout, s); err == nil {
			return tm, true
		}
	}
	return time.Time{}, false
}

// Tailer reads and optionally follows one or more log files.
type Tailer struct {
	paths    []string
	interval time.Duration
}

// NewTailer creates a Tailer for the given file paths.
// interval controls how often files are polled in follow mode (default 500ms).
func NewTailer(paths []string, interval time.Duration) *Tailer {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &Tailer{paths: paths, interval: interval}
}

// ReadInitial reads the last n lines from each file and returns them
// concatenated with a file header separator when multiple files are given.
// If n <= 0, all lines are returned.
func (t *Tailer) ReadInitial(n int) ([]string, error) {
	var all []string
	for i, p := range t.paths {
		lines, err := readLastLines(p, n)
		if err != nil {
			if i > 0 {
				// Continue with other files; report error on stderr via fmt later.
				all = append(all, "  [error reading "+p+": "+err.Error()+"]")
			} else {
				return nil, err
			}
		}
		if len(t.paths) > 1 {
			all = append(all, "── "+p+" ──")
		}
		all = append(all, lines...)
	}
	return all, nil
}

// Follow blocks until ctx is cancelled, emitting new lines to ch as they
// appear. It polls each file every t.interval.
func (t *Tailer) Follow(ctx context.Context, ch chan<- string) {
	defer close(ch)

	offsets := make(map[string]int64, len(t.paths))
	for _, p := range t.paths {
		offsets[p] = fileSize(p)
	}

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, p := range t.paths {
				info, err := os.Stat(p)
				if err != nil {
					continue
				}
				// File was truncated/rotated: reset offset.
				if info.Size() < offsets[p] {
					offsets[p] = 0
				}
				if info.Size() == offsets[p] {
					continue
				}
				newLines, err := readFromOffset(p, offsets[p])
				if err == nil {
					offsets[p] = info.Size()
					for _, l := range newLines {
						select {
						case ch <- l:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
	}
}

// readLastLines returns the last n lines of the file at path.
// If n <= 0, all lines are returned.
func readLastLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	// For small files just read everything.
	if size < 1<<20 { // 1 MiB
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		lines := splitLines(string(data))
		if n > 0 && len(lines) > n {
			lines = lines[len(lines)-n:]
		}
		return lines, nil
	}

	// For large files, seek to near the end and read backwards.
	// Read up to 4 MiB from the end, which should be more than enough
	// for the requested number of lines.
	const maxRead = 4 << 20
	start := size - maxRead
	if start < 0 {
		start = 0
	}
	f.Seek(start, 0)

	buf := make([]byte, size-start)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	// Discard the first partial line if we didn't start at 0.
	if start > 0 {
		firstNL := strings.IndexByte(string(buf), '\n')
		if firstNL >= 0 {
			buf = buf[firstNL+1:]
		}
	}
	lines := splitLines(string(buf))
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// readFromOffset reads new content from a file starting at the given byte offset.
func readFromOffset(path string, offset int64) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // up to 1 MiB per line
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// splitLines splits a string into lines, trimming trailing newlines.
func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// fileSize returns the size of a file, or 0 on error.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// FilterOptions configures line filtering.
type FilterOptions struct {
	MinLevel Level
	Since    time.Duration
	Now      time.Time // reference "now" for Since; defaults to time.Now()
	Grep     *regexp.Regexp
}

// FilterLines returns only the lines that pass all configured filters.
func FilterLines(lines []string, opts FilterOptions) []string {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	cutoff := now.Add(-opts.Since)

	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := ParseLine(raw)

		// Level filter.
		if opts.MinLevel > LevelUnknown && line.Level > LevelUnknown && line.Level < opts.MinLevel {
			continue
		}

		// Time filter.
		if opts.Since > 0 && line.HasTime && line.Time.Before(cutoff) {
			continue
		}

		// Grep filter.
		if opts.Grep != nil && !opts.Grep.MatchString(raw) {
			continue
		}

		out = append(out, raw)
	}
	return out
}

