package patch

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// diffContext is the number of unchanged lines shown around a change. Two
// changes whose unchanged gap is at most 2*diffContext share one hunk, which is
// the same threshold as diff -U3.
const diffContext = 3

// FromTexts builds a one-file unified diff from two texts.
//
// The result can be rendered with Render or Lines, or opened in the interactive
// viewer. Identical texts yield no files. A NUL byte on either side yields a
// binary-file notice instead of hunks, because a line diff of binary data would
// be nonsense.
//
// The diff is produced in process: no git or diff process is started. Line
// endings are treated as in Parse, so CRLF input renders without the carriage
// return, and a missing final newline is marked with a NoNewline line.
func FromTexts(oldPath, newPath string, oldText, newText []byte) []File {
	if bytes.Equal(oldText, newText) {
		return nil
	}

	f := File{OldPath: oldPath, NewPath: newPath}
	if bytes.IndexByte(oldText, 0) >= 0 || bytes.IndexByte(newText, 0) >= 0 {
		f.Binary = true
		f.Header = []string{fmt.Sprintf("Binary files %s and %s differ", oldPath, newPath)}
		return []File{f}
	}

	oldLines := splitLinesKeepEnds(oldText)
	newLines := splitLinesKeepEnds(newText)
	chunks := lineChunks(oldLines, newLines)
	if len(chunks) == 0 {
		return nil
	}

	f.OldLine = "--- " + oldPath
	f.NewLine = "+++ " + newPath
	f.Hunks = buildHunks(oldLines, newLines, chunks)
	return []File{f}
}

// lineChunk is one contiguous change: old[oldStart:oldEnd) was replaced by
// new[newStart:newEnd). A pure insertion has oldStart == oldEnd; a pure deletion
// has newStart == newEnd.
type lineChunk struct {
	oldStart, oldEnd int
	newStart, newEnd int
}

// lineChunks returns the line-level changes that turn oldLines into newLines.
//
// go-diff encodes one line as one rune, so the rune count of a diff piece is its
// line count. The encoding is never decoded: only the line indices are needed.
func lineChunks(oldLines, newLines []string) []lineChunk {
	if len(oldLines) == 0 && len(newLines) == 0 {
		return nil
	}

	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 2 * time.Second
	encOld, encNew, _ := dmp.DiffLinesToChars(strings.Join(oldLines, ""), strings.Join(newLines, ""))
	diffs := dmp.DiffMain(encOld, encNew, false)

	var chunks []lineChunk
	oldIdx, newIdx := 0, 0
	var cur lineChunk
	open := false
	closeChunk := func() {
		if !open {
			return
		}
		cur.oldEnd = oldIdx
		cur.newEnd = newIdx
		chunks = append(chunks, cur)
		open = false
	}

	for _, d := range diffs {
		n := utf8.RuneCountInString(d.Text)
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			closeChunk()
			oldIdx += n
			newIdx += n
		case diffmatchpatch.DiffDelete:
			if !open {
				cur = lineChunk{oldStart: oldIdx, newStart: newIdx}
				open = true
			}
			oldIdx += n
		case diffmatchpatch.DiffInsert:
			if !open {
				cur = lineChunk{oldStart: oldIdx, newStart: newIdx}
				open = true
			}
			newIdx += n
		}
	}
	closeChunk()
	return chunks
}

// buildHunks groups chunks into @@ blocks with diffContext lines of context,
// merging neighbours whose unchanged gap is at most 2*diffContext.
func buildHunks(oldLines, newLines []string, chunks []lineChunk) []Hunk {
	oldMissingNL := missingFinalNewline(oldLines)
	newMissingNL := missingFinalNewline(newLines)

	var hunks []Hunk
	for i := 0; i < len(chunks); {
		j := i
		for j+1 < len(chunks) && chunks[j+1].oldStart-chunks[j].oldEnd <= 2*diffContext {
			j++
		}

		start := max(0, chunks[i].oldStart-diffContext)
		endOld := min(len(oldLines), chunks[j].oldEnd+diffContext)

		var b hunkBuilder
		b.oldMissingNL, b.newMissingNL = oldMissingNL, newMissingNL

		oldIdx := start
		newIdx := chunks[i].newStart - (chunks[i].oldStart - start)
		for oldIdx < chunks[i].oldStart {
			b.context(oldLines[oldIdx], oldIdx == len(oldLines)-1, newIdx == len(newLines)-1)
			oldIdx++
			newIdx++
		}
		for k := i; k <= j; k++ {
			c := chunks[k]
			for oldIdx < c.oldStart {
				b.context(oldLines[oldIdx], oldIdx == len(oldLines)-1, newIdx == len(newLines)-1)
				oldIdx++
				newIdx++
			}
			for oldIdx < c.oldEnd {
				b.delete(oldLines[oldIdx], oldIdx == len(oldLines)-1)
				oldIdx++
			}
			for newIdx < c.newEnd {
				b.add(newLines[newIdx], newIdx == len(newLines)-1)
				newIdx++
			}
		}
		for oldIdx < endOld {
			b.context(oldLines[oldIdx], oldIdx == len(oldLines)-1, newIdx == len(newLines)-1)
			oldIdx++
			newIdx++
		}

		h := Hunk{Lines: b.lines}
		h.OldStart = start + 1
		h.OldCount = oldIdx - start
		h.NewStart = chunks[i].newStart - (chunks[i].oldStart - start) + 1
		h.NewCount = newIdx - (h.NewStart - 1)
		// A zero-length side is written as "-0,0"/"+0,0" at the start of the
		// file, matching diff -u.
		if h.OldCount == 0 {
			h.OldStart = 0
		}
		if h.NewCount == 0 {
			h.NewStart = 0
		}
		h.Header = fmt.Sprintf("@@ -%s +%s @@", hunkRange(h.OldStart, h.OldCount), hunkRange(h.NewStart, h.NewCount))

		hunks = append(hunks, h)
		i = j + 1
	}
	return hunks
}

// hunkBuilder accumulates hunk lines and appends "\ No newline at end of file"
// markers where a displayed line is the last line of a side that lacks one.
type hunkBuilder struct {
	lines        []Line
	oldMissingNL bool
	newMissingNL bool
}

func (b *hunkBuilder) context(text string, oldLast, newLast bool) {
	b.lines = append(b.lines, Line{Kind: Context, Text: cleanLine(text)})
	b.marker(oldLast, newLast)
}

func (b *hunkBuilder) delete(text string, oldLast bool) {
	b.lines = append(b.lines, Line{Kind: Delete, Text: cleanLine(text)})
	b.marker(oldLast, false)
}

func (b *hunkBuilder) add(text string, newLast bool) {
	b.lines = append(b.lines, Line{Kind: Add, Text: cleanLine(text)})
	b.marker(false, newLast)
}

func (b *hunkBuilder) marker(oldLast, newLast bool) {
	if !(oldLast && b.oldMissingNL) && !(newLast && b.newMissingNL) {
		return
	}
	// A context line at EOF belongs to both sides: one marker is enough.
	if n := len(b.lines); n > 0 && b.lines[n-1].Kind == NoNewline {
		return
	}
	b.lines = append(b.lines, Line{Kind: NoNewline})
}

// hunkRange formats one side of a hunk header, omitting a count of one.
func hunkRange(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

// splitLinesKeepEnds splits text into lines that keep their trailing newline, so
// a missing newline at EOF stays observable. An empty text has no lines.
func splitLinesKeepEnds(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func missingFinalNewline(lines []string) bool {
	return len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n")
}

// cleanLine drops the terminator and a CRLF carriage return, matching the line
// text Parse produces for patch input.
func cleanLine(line string) string {
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r")
}
