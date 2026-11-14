// Package patch parses and renders unified diffs.
//
// It understands both `git diff` output and plain `diff -u` output, and treats
// the patch as text: no git process is started and no repository is required.
package patch

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind is the role of one line inside a hunk.
type Kind int

const (
	// Context is an unchanged line.
	Context Kind = iota
	// Add is a line only the new side has.
	Add
	// Delete is a line only the old side has.
	Delete
	// NoNewline marks "\ No newline at end of file" and carries no content.
	NoNewline
)

// Line is one line of a hunk.
type Line struct {
	Kind Kind
	Text string
}

// Hunk is one @@ block.
type Hunk struct {
	Header   string
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []Line
}

// File is one file section of a patch.
type File struct {
	// Header holds the raw metadata lines before the hunks: "diff --git",
	// "index", modes, rename/copy lines, binary notices.
	Header  []string
	OldLine string // raw "--- " line, empty when the patch has none
	NewLine string // raw "+++ " line, empty when the patch has none
	OldPath string // display path without the a/ or b/ prefix
	NewPath string
	Binary  bool
	Hunks   []Hunk
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// Parse turns unified diff text into file sections.
//
// Lines outside a file section (an email header, a commit message, a diffstat)
// are ignored. Combined diffs are rejected rather than rendered incorrectly.
func Parse(data []byte) ([]File, error) {
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("input looks like a binary file, not a unified diff")
	}

	lines := splitLines(data)
	var files []File
	var cur *File
	inBinaryPayload := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if inBinaryPayload {
			// The base85 payload has no structure; only a new file header ends it.
			if strings.HasPrefix(line, "diff --git ") {
				inBinaryPayload = false
			} else {
				continue
			}
		}

		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, File{})
			cur = &files[len(files)-1]
			cur.Header = append(cur.Header, line)
			if oldPath, newPath, ok := splitPathPair(strings.TrimPrefix(line, "diff --git ")); ok {
				cur.OldPath = stripPrefix(oldPath)
				cur.NewPath = stripPrefix(newPath)
			}

		case strings.HasPrefix(line, "diff --cc "), strings.HasPrefix(line, "diff --combined "):
			return nil, fmt.Errorf("line %d: combined diffs are not supported", i+1)

		case strings.HasPrefix(line, "--- "):
			// A plain diff has no "diff --git" line: the file starts here. In git
			// format this line belongs to the file that already started.
			if cur == nil || len(cur.Hunks) > 0 || cur.NewLine != "" {
				files = append(files, File{})
				cur = &files[len(files)-1]
			}
			cur.OldLine = line
			cur.OldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))

		case strings.HasPrefix(line, "+++ "):
			if cur == nil {
				files = append(files, File{})
				cur = &files[len(files)-1]
			}
			cur.NewLine = line
			cur.NewPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))

		case strings.HasPrefix(line, "@@@"):
			return nil, fmt.Errorf("line %d: combined diffs are not supported", i+1)

		case strings.HasPrefix(line, "@@"):
			if cur == nil {
				return nil, fmt.Errorf("line %d: hunk without a file header", i+1)
			}
			hunk, next, err := parseHunk(lines, i)
			if err != nil {
				return nil, err
			}
			cur.Hunks = append(cur.Hunks, hunk)
			i = next - 1

		case strings.HasPrefix(line, "GIT binary patch"):
			if cur != nil {
				cur.Binary = true
				cur.Header = append(cur.Header, line)
			}
			inBinaryPayload = true

		case strings.HasPrefix(line, "Binary files "):
			if cur == nil {
				files = append(files, File{})
				cur = &files[len(files)-1]
			}
			cur.Binary = true
			cur.Header = append(cur.Header, line)
			if oldPath, newPath, ok := binaryNoticePaths(line); ok {
				if cur.OldPath == "" {
					cur.OldPath = stripPrefix(oldPath)
				}
				if cur.NewPath == "" {
					cur.NewPath = stripPrefix(newPath)
				}
			}

		default:
			if cur != nil {
				cur.Header = append(cur.Header, line)
			}
			// Text before the first file (mail headers, commit message) is ignored.
		}
	}

	return files, nil
}

// parseHunk parses a hunk header and exactly OldCount/NewCount body lines.
func parseHunk(lines []string, idx int) (Hunk, int, error) {
	m := hunkHeader.FindStringSubmatch(lines[idx])
	if m == nil {
		return Hunk{}, idx, fmt.Errorf("line %d: invalid hunk header %q", idx+1, lines[idx])
	}

	h := Hunk{Header: lines[idx]}
	h.OldStart, _ = strconv.Atoi(m[1])
	h.OldCount = countOrDefault(m[2])
	h.NewStart, _ = strconv.Atoi(m[3])
	h.NewCount = countOrDefault(m[4])

	oldLeft, newLeft := h.OldCount, h.NewCount
	i := idx + 1
	for oldLeft > 0 || newLeft > 0 {
		if i >= len(lines) {
			return Hunk{}, i, fmt.Errorf("line %d: hunk ended early: %d old and %d new line(s) missing", i, oldLeft, newLeft)
		}
		line := lines[i]
		switch {
		case line == "" || strings.HasPrefix(line, " "):
			// An empty line is malformed but common; treat it as empty context.
			h.Lines = append(h.Lines, Line{Kind: Context, Text: strings.TrimPrefix(line, " ")})
			oldLeft--
			newLeft--
		case strings.HasPrefix(line, "+"):
			h.Lines = append(h.Lines, Line{Kind: Add, Text: line[1:]})
			newLeft--
		case strings.HasPrefix(line, "-"):
			h.Lines = append(h.Lines, Line{Kind: Delete, Text: line[1:]})
			oldLeft--
		case strings.HasPrefix(line, `\`):
			h.Lines = append(h.Lines, Line{Kind: NoNewline})
		default:
			return Hunk{}, i, fmt.Errorf("line %d: unexpected line in hunk: %q", i+1, line)
		}
		i++
	}

	// Consume "\ No newline at end of file" markers that follow the last line.
	for i < len(lines) && strings.HasPrefix(lines[i], `\`) {
		h.Lines = append(h.Lines, Line{Kind: NoNewline})
		i++
	}

	return h, i, nil
}

func countOrDefault(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// splitLines splits on line endings, dropping the terminators. CRLF is accepted
// as a line separator so patches produced on Windows render cleanly.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		lines[i] = line
	}
	return lines
}

// parseDiffPath extracts the path from a "--- " or "+++ " line, dropping the
// tab-separated timestamp that plain diff writes and unquoting git's path form.
func parseDiffPath(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	return stripPrefix(unquoteGitPath(strings.TrimSpace(s)))
}

func stripPrefix(path string) string {
	if path == "/dev/null" {
		return path
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

func unquoteGitPath(path string) string {
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		if unquoted, err := strconv.Unquote(path); err == nil {
			return unquoted
		}
	}
	return path
}

// splitPathPair splits the two paths of a "diff --git a/x b/x" header, honouring
// git's quoted form for paths with spaces or special characters.
func splitPathPair(s string) (string, string, bool) {
	if s == "" {
		return "", "", false
	}
	if s[0] == '"' {
		first, rest, ok := consumeQuoted(s)
		if !ok {
			return "", "", false
		}
		second, _, ok := consumeQuoted(strings.TrimLeft(rest, " "))
		if !ok {
			return "", "", false
		}
		return first, second, true
	}
	if i := strings.IndexByte(s, ' '); i > 0 {
		second := strings.TrimSpace(s[i+1:])
		if second != "" {
			return s[:i], second, true
		}
	}
	return "", "", false
}

func consumeQuoted(s string) (string, string, bool) {
	if len(s) == 0 || s[0] != '"' {
		return "", "", false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			value, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return "", "", false
			}
			return value, s[i+1:], true
		}
	}
	return "", "", false
}

// binaryNoticePaths parses "Binary files a/x and b/x differ".
func binaryNoticePaths(line string) (string, string, bool) {
	rest := strings.TrimSuffix(strings.TrimPrefix(line, "Binary files "), " differ")
	i := strings.Index(rest, " and ")
	if i <= 0 {
		return "", "", false
	}
	return rest[:i], rest[i+len(" and "):], true
}
