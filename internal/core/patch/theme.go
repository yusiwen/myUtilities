package patch

import (
	"fmt"
	"strconv"
	"strings"
)

// Theme selects the colour palette used by the renderer and the TUI.
type Theme int

const (
	// ThemeDefault keeps the traditional 16-colour diff palette.
	ThemeDefault Theme = iota
	// ThemeNord uses the Nord palette (https://www.nordtheme.com).
	ThemeNord
)

// Nord palette values, named north0..nord15 in the official scheme.
const (
	Nord0  = "#2E3440"
	Nord1  = "#3B4252"
	Nord2  = "#434C5E"
	Nord3  = "#4C566A"
	Nord4  = "#D8DEE9"
	Nord5  = "#E5E9F0"
	Nord6  = "#ECEFF4"
	Nord7  = "#8FBCBB"
	Nord8  = "#88C0D0"
	Nord9  = "#81A1C1"
	Nord10 = "#5E81AC"
	Nord11 = "#BF616A"
	Nord12 = "#D08770"
	Nord13 = "#EBCB8B"
	Nord14 = "#A3BE8C"
	Nord15 = "#B48EAD"
)

// palette holds the ANSI prefix for each rendered line kind. The reset suffix is
// appended by paint.
type palette struct {
	header    string
	meta      string
	oldPath   string
	newPath   string
	hunk      string
	context   string
	add       string
	delete    string
	noNewline string
	binary    string
	gutter    string
}

func paletteFor(theme Theme) palette {
	if theme == ThemeNord {
		return palette{
			header:    ansiBold + fgHex(Nord6),
			meta:      fgHex(Nord3),
			oldPath:   fgHex(Nord11),
			newPath:   fgHex(Nord14),
			hunk:      ansiBold + fgHex(Nord8),
			context:   ansiDim + fgHex(Nord4),
			add:       fgHex(Nord14),
			delete:    fgHex(Nord11),
			noNewline: fgHex(Nord3),
			binary:    fgHex(Nord13),
			gutter:    fgHex(Nord3),
		}
	}
	return palette{
		header:    ansiBold,
		meta:      ansiDim,
		oldPath:   ansiRed,
		newPath:   ansiGreen,
		hunk:      ansiBold + ansiCyan,
		context:   ansiDim,
		add:       ansiGreen,
		delete:    ansiRed,
		noNewline: ansiDim,
		binary:    ansiYellow,
		gutter:    ansiDim,
	}
}

// color returns the ANSI prefix for a line kind.
func (p palette) color(kind LineKind) string {
	switch kind {
	case LineHeader:
		return p.header
	case LineMeta:
		return p.meta
	case LineOldPath:
		return p.oldPath
	case LineNewPath:
		return p.newPath
	case LineHunk:
		return p.hunk
	case LineContext:
		return p.context
	case LineAdd:
		return p.add
	case LineDelete:
		return p.delete
	case LineNoNewline:
		return p.noNewline
	case LineBinary:
		return p.binary
	default:
		return ""
	}
}

// fgHex converts "#RRGGBB" to a 24-bit ANSI foreground prefix. It returns "" for
// an unparsable value, which callers treat as "no colour".
func fgHex(hex string) string {
	value, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", value>>16, (value>>8)&0xff, value&0xff)
}

// Paint wraps text in the theme's colour for a line kind. It is used by the
// two-way text renderer (`mu diff file` / `mu diff text`), which has no gutter.
func Paint(theme Theme, kind LineKind, text string) string {
	prefix := paletteFor(theme).color(kind)
	if prefix == "" || text == "" {
		return text
	}
	return prefix + text + ansiReset
}
