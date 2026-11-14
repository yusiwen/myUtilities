package patch

import "strings"

// LineKind classifies a rendered line so a caller can colour it without parsing
// the text back.
type LineKind int

const (
	// LineHeader is the "diff --git" line.
	LineHeader LineKind = iota
	// LineMeta is index/mode/rename metadata.
	LineMeta
	// LineOldPath is the raw "--- " line.
	LineOldPath
	// LineNewPath is the raw "+++ " line.
	LineNewPath
	// LineHunk is an "@@" header.
	LineHunk
	// LineContext is an unchanged line.
	LineContext
	// LineAdd is an added line.
	LineAdd
	// LineDelete is a deleted line.
	LineDelete
	// LineNoNewline is the "\ No newline at end of file" marker.
	LineNoNewline
	// LineBinary is a binary-file notice.
	LineBinary
)

// DisplayLine is one rendered line of a file section: no colour, no trailing
// newline. Gutter is the line-number prefix (empty for headers and metadata),
// so a caller can style it separately from the content.
type DisplayLine struct {
	Gutter string
	Text   string
	Kind   LineKind
}

// Lines renders one file section in plain text. It is the shared source of
// truth for the plain renderer and the interactive viewer.
func Lines(f File, opts Options) []DisplayLine {
	var out []DisplayLine

	for _, line := range f.Header {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			out = append(out, DisplayLine{Text: line, Kind: LineHeader})
		case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
			out = append(out, DisplayLine{Text: line, Kind: LineBinary})
		default:
			out = append(out, DisplayLine{Text: line, Kind: LineMeta})
		}
	}
	if f.OldLine != "" {
		out = append(out, DisplayLine{Text: f.OldLine, Kind: LineOldPath})
	}
	if f.NewLine != "" {
		out = append(out, DisplayLine{Text: f.NewLine, Kind: LineNewPath})
	}

	oldWidth, newWidth := lineWidths(&f)
	for _, h := range f.Hunks {
		out = append(out, DisplayLine{Text: h.Header, Kind: LineHunk})
		oldNo, newNo := h.OldStart, h.NewStart
		for _, line := range h.Lines {
			switch line.Kind {
			case Context:
				gutter, body := contentParts(opts, oldWidth, newWidth, oldNo, newNo, ' ', line.Text)
				out = append(out, DisplayLine{Gutter: gutter, Text: body, Kind: LineContext})
				oldNo++
				newNo++
			case Delete:
				gutter, body := contentParts(opts, oldWidth, newWidth, oldNo, -1, '-', line.Text)
				out = append(out, DisplayLine{Gutter: gutter, Text: body, Kind: LineDelete})
				oldNo++
			case Add:
				gutter, body := contentParts(opts, oldWidth, newWidth, -1, newNo, '+', line.Text)
				out = append(out, DisplayLine{Gutter: gutter, Text: body, Kind: LineAdd})
				newNo++
			case NoNewline:
				out = append(out, DisplayLine{Text: noNewlineLine(opts, oldWidth, newWidth), Kind: LineNoNewline})
			}
		}
	}
	return out
}

// contentParts returns the gutter and the marker-prefixed content of one line.
func contentParts(opts Options, oldWidth, newWidth, oldNo, newNo int, marker byte, text string) (string, string) {
	gutter := ""
	if opts.LineNumbers {
		gutter = gutterText(oldNo, newNo, oldWidth, newWidth)
	}
	return gutter, string(marker) + text
}
