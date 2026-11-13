// Package qr renders QR codes for terminal display.
//
// The terminal renderer deliberately does not rely on the user's colour theme:
// a QR code is only scannable with real contrast between light and dark
// modules, so every module is painted with an explicit background colour.
package qr

import (
	"fmt"
	"strings"

	gqrcode "github.com/skip2/go-qrcode"
)

// ANSI SGR sequences painted behind each module. Palette entries 16 (black) and
// 231 (white) are supported by every colour terminal, unlike truecolor escapes.
const (
	ansiBlack = "\x1b[48;5;16m"
	ansiWhite = "\x1b[48;5;231m"
	ansiReset = "\x1b[0m"
)

// Rendered is a QR code drawn as terminal text.
type Rendered struct {
	// Art is the drawing itself, one terminal row per line.
	Art string
	// Width is the number of terminal columns the drawing occupies.
	Width int
	// Height is the number of terminal rows the drawing occupies.
	Height int
}

// Terminal renders text as a QR code that can be scanned from a terminal.
//
// Each module is drawn as two spaces with its own background colour, so the
// result keeps black-on-white contrast on both light and dark terminal themes.
// The four-module quiet zone included in the bitmap is drawn as well, which is
// what a scanner needs to lock onto the code.
func Terminal(text string, level gqrcode.RecoveryLevel) (*Rendered, error) {
	code, err := gqrcode.New(text, level)
	if err != nil {
		return nil, fmt.Errorf("encode qr: %w", err)
	}
	bitmap := code.Bitmap()
	if len(bitmap) == 0 {
		return nil, fmt.Errorf("encode qr: empty bitmap")
	}

	var b strings.Builder
	for _, row := range bitmap {
		current := ansiWhite
		b.WriteString(current)
		for _, dark := range row {
			want := ansiWhite
			if dark {
				want = ansiBlack
			}
			// Collapse runs of the same colour into one escape sequence; a
			// per-module escape would add several kilobytes of noise.
			if want != current {
				b.WriteString(want)
				current = want
			}
			b.WriteString("  ")
		}
		b.WriteString(ansiReset)
		b.WriteByte('\n')
	}

	return &Rendered{
		Art:    b.String(),
		Width:  len(bitmap[0]) * 2,
		Height: len(bitmap),
	}, nil
}
