// Package merge implements a line-based three-way text merge.
//
// The engine is deliberately independent of git: it takes three byte slices
// (common ancestor, ours, theirs) and returns the merged text plus the number of
// conflict regions. Callers decide where the inputs come from.
package merge

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Style selects the conflict marker layout.
type Style int

const (
	// StyleMerge writes two-sided markers: <<<<<<< / ======= / >>>>>>>.
	StyleMerge Style = iota
	// StyleDiff3 also writes the common ancestor between ||||||| and =======.
	StyleDiff3
)

// Labels are written after the conflict markers. Empty labels fall back to
// "ours", "base" and "theirs".
type Labels struct {
	Ours   string
	Base   string
	Theirs string
}

// Options configures Merge.
type Options struct {
	Style      Style
	Labels     Labels
	MarkerSize int
}

// Result is the merged document.
type Result struct {
	Text      []byte
	Conflicts int
}

// ErrInvalidOptions is wrapped by every option validation error.
var ErrInvalidOptions = errors.New("invalid merge options")

const defaultMarkerSize = 7

// Merge performs a three-way merge of base, ours and theirs.
func Merge(base, ours, theirs []byte, opts Options) (Result, error) {
	if opts.Style != StyleMerge && opts.Style != StyleDiff3 {
		return Result{}, fmt.Errorf("%w: unknown style %d", ErrInvalidOptions, opts.Style)
	}
	if opts.MarkerSize < 0 {
		return Result{}, fmt.Errorf("%w: marker size must not be negative", ErrInvalidOptions)
	}
	if opts.MarkerSize == 0 {
		opts.MarkerSize = defaultMarkerSize
	}

	labels := opts.Labels
	if labels.Ours == "" {
		labels.Ours = "ours"
	}
	if labels.Base == "" {
		labels.Base = "base"
	}
	if labels.Theirs == "" {
		labels.Theirs = "theirs"
	}

	baseLines := splitLines(base)
	oursLines := splitLines(ours)
	theirsLines := splitLines(theirs)

	oursChanges := diffChunks(baseLines, oursLines)
	theirsChanges := diffChunks(baseLines, theirsLines)

	text, conflicts := mergeLines(baseLines, oursLines, theirsLines, oursChanges, theirsChanges,
		opts.Style, labels, opts.MarkerSize)
	return Result{Text: []byte(text), Conflicts: conflicts}, nil
}

// splitLines splits text into lines, each keeping its trailing newline. A final
// line without a newline stays without one, so merging never invents a newline.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// chunk is one contiguous change: base[start:end) was replaced by
// side[sideStart:sideEnd). A pure insertion has start == end.
type chunk struct {
	baseStart, baseEnd int
	sideStart, sideEnd int
}

// diffChunks returns the line-level changes that turn base into side.
//
// Lines are encoded one rune per line by go-diff, so the rune count of a diff
// piece is its line count. The encoding is left in place: the merge only needs
// indices, never the diff text itself.
func diffChunks(base, side []string) []chunk {
	if len(base) == 0 && len(side) == 0 {
		return nil
	}

	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 2 * time.Second
	encBase, encSide, _ := dmp.DiffLinesToChars(strings.Join(base, ""), strings.Join(side, ""))
	diffs := dmp.DiffMain(encBase, encSide, false)

	var chunks []chunk
	baseIdx, sideIdx := 0, 0
	open := false
	var cur chunk
	closeChunk := func() {
		if !open {
			return
		}
		cur.baseEnd = baseIdx
		cur.sideEnd = sideIdx
		chunks = append(chunks, cur)
		open = false
	}

	for _, d := range diffs {
		n := utf8.RuneCountInString(d.Text)
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			closeChunk()
			baseIdx += n
			sideIdx += n
		case diffmatchpatch.DiffDelete:
			if !open {
				cur = chunk{baseStart: baseIdx, sideStart: sideIdx}
				open = true
			}
			baseIdx += n
		case diffmatchpatch.DiffInsert:
			if !open {
				cur = chunk{baseStart: baseIdx, sideStart: sideIdx}
				open = true
			}
			sideIdx += n
		}
	}
	closeChunk()
	return chunks
}

// mergeLines walks the base line by line, combining both sides' change chunks
// into regions. Chunks that touch (overlap or abut) form one region, matching
// diff3 and git merge-file: editing adjacent lines is treated as a conflict
// instead of risking an interleave neither side asked for.
func mergeLines(base, ours, theirs []string, oursChanges, theirsChanges []chunk,
	style Style, labels Labels, markerSize int) (string, int) {

	var out []string
	pos := 0
	ia, ib := 0, 0
	conflicts := 0

	for ia < len(oursChanges) || ib < len(theirsChanges) {
		start := nextStart(oursChanges, ia, theirsChanges, ib)
		end := start
		var regionOurs, regionTheirs []chunk

		for {
			progress := false
			if ia < len(oursChanges) && oursChanges[ia].baseStart <= end {
				end = max(end, oursChanges[ia].baseEnd)
				regionOurs = append(regionOurs, oursChanges[ia])
				ia++
				progress = true
			}
			if ib < len(theirsChanges) && theirsChanges[ib].baseStart <= end {
				end = max(end, theirsChanges[ib].baseEnd)
				regionTheirs = append(regionTheirs, theirsChanges[ib])
				ib++
				progress = true
			}
			if !progress {
				break
			}
		}

		out = append(out, base[pos:start]...)

		oursText := applyChunks(base, ours, regionOurs, start, end)
		theirsText := applyChunks(base, theirs, regionTheirs, start, end)

		switch {
		case len(regionTheirs) == 0:
			out = append(out, oursText...)
		case len(regionOurs) == 0:
			out = append(out, theirsText...)
		case equalLines(oursText, theirsText):
			// Both sides made the same change; write it once.
			out = append(out, oursText...)
		default:
			out = appendConflict(out, oursText, base[start:end], theirsText, style, labels, markerSize)
			conflicts++
		}
		pos = end
	}

	out = append(out, base[pos:]...)
	return strings.Join(out, ""), conflicts
}

// nextStart is the earliest base position either side changes next.
func nextStart(a []chunk, ia int, b []chunk, ib int) int {
	start := -1
	if ia < len(a) {
		start = a[ia].baseStart
	}
	if ib < len(b) && (start < 0 || b[ib].baseStart < start) {
		start = b[ib].baseStart
	}
	return start
}

// applyChunks returns side's view of base[start:end): unchanged base lines with
// that side's chunks applied. Insertions exactly at end belong to the region and
// are placed after the base lines, matching diff3.
func applyChunks(base, side []string, chunks []chunk, start, end int) []string {
	var out []string
	pos := start
	for _, c := range chunks {
		out = append(out, base[pos:c.baseStart]...)
		out = append(out, side[c.sideStart:c.sideEnd]...)
		pos = c.baseEnd
	}
	out = append(out, base[pos:end]...)
	return out
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func appendConflict(out, oursText, baseText, theirsText []string,
	style Style, labels Labels, markerSize int) []string {

	out = appendMarker(out, strings.Repeat("<", markerSize), labels.Ours)
	out = append(out, oursText...)
	if style == StyleDiff3 {
		out = appendMarker(out, strings.Repeat("|", markerSize), labels.Base)
		out = append(out, baseText...)
	}
	out = appendMarker(out, strings.Repeat("=", markerSize), "")
	out = append(out, theirsText...)
	out = appendMarker(out, strings.Repeat(">", markerSize), labels.Theirs)
	return out
}

// appendMarker writes a marker on its own line. A previous line without a final
// newline is terminated first, which is what git merge-file does when a conflict
// sits at end of file.
func appendMarker(out []string, marker, label string) []string {
	if len(out) > 0 && !strings.HasSuffix(out[len(out)-1], "\n") {
		out[len(out)-1] += "\n"
	}
	line := marker
	if label != "" {
		line += " " + label
	}
	return append(out, line+"\n")
}
