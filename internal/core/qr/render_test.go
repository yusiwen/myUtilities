package qr

import (
	"regexp"
	"strings"
	"testing"

	gqrcode "github.com/skip2/go-qrcode"
)

// ansiEscape matches the SGR sequences emitted by the renderer.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visibleWidth returns the number of terminal columns a rendered line occupies.
// Every module is two spaces wide, so the rune count is the column count.
func visibleWidth(line string) int {
	return len([]rune(ansiEscape.ReplaceAllString(line, "")))
}

func TestTerminalShape(t *testing.T) {
	r, err := Terminal("http://192.168.1.5:8080/", gqrcode.Medium)
	if err != nil {
		t.Fatalf("Terminal: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(r.Art, "\n"), "\n")
	if len(lines) != r.Height {
		t.Fatalf("got %d lines, Rendered.Height = %d", len(lines), r.Height)
	}
	if r.Width%2 != 0 {
		t.Errorf("Width = %d, want an even number of columns (two per module)", r.Width)
	}
	// The bitmap is square, so two columns per module makes width exactly twice
	// the height.
	if r.Width != 2*r.Height {
		t.Errorf("Width = %d, Height = %d: want Width == 2*Height", r.Width, r.Height)
	}
	for i, line := range lines {
		if got := visibleWidth(line); got != r.Width {
			t.Fatalf("line %d has %d columns, want %d", i, got, r.Width)
		}
		if !strings.HasSuffix(line, ansiReset) {
			t.Fatalf("line %d does not reset the colour, which would bleed into the next line", i)
		}
	}
}

func TestTerminalForcesBlackAndWhite(t *testing.T) {
	r, err := Terminal("http://192.168.1.5:8080/", gqrcode.Medium)
	if err != nil {
		t.Fatalf("Terminal: %v", err)
	}
	if !strings.Contains(r.Art, ansiBlack) {
		t.Error("drawing contains no black module")
	}
	if !strings.Contains(r.Art, ansiWhite) {
		t.Error("drawing contains no white module")
	}

	// The quiet zone is drawn explicitly, so the first row must start white even
	// on a dark terminal theme.
	firstLine := strings.SplitN(r.Art, "\n", 2)[0]
	if !strings.HasPrefix(firstLine, ansiWhite) {
		t.Errorf("first line starts with %q, want the white quiet zone", firstLine[:min(len(firstLine), 16)])
	}
}

func TestTerminalIsDeterministic(t *testing.T) {
	a, err := Terminal("hello", gqrcode.Medium)
	if err != nil {
		t.Fatalf("Terminal: %v", err)
	}
	b, err := Terminal("hello", gqrcode.Medium)
	if err != nil {
		t.Fatalf("Terminal: %v", err)
	}
	if a.Art != b.Art {
		t.Error("same input rendered differently")
	}
}

func TestTerminalErrorCorrectionGrowsCode(t *testing.T) {
	low, err := Terminal("http://192.168.1.5:8080/", gqrcode.Low)
	if err != nil {
		t.Fatalf("Terminal(low): %v", err)
	}
	high, err := Terminal("http://192.168.1.5:8080/", gqrcode.High)
	if err != nil {
		t.Fatalf("Terminal(high): %v", err)
	}
	if high.Height <= low.Height {
		t.Errorf("high error correction produced %d rows, low produced %d; want a larger symbol", high.Height, low.Height)
	}
}

// parseArt rebuilds the module matrix from the drawing, so a test can compare
// what a terminal will show against the symbol the encoder produced.
func parseArt(t *testing.T, art string) [][]bool {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(art, "\n"), "\n")
	rows := make([][]bool, 0, len(lines))
	for i, line := range lines {
		var row []bool
		dark := false
		for len(line) > 0 {
			if strings.HasPrefix(line, "\x1b[") {
				end := strings.IndexByte(line, 'm')
				if end < 0 {
					t.Fatalf("line %d: unterminated escape", i)
				}
				switch seq := line[:end+1]; seq {
				case ansiBlack:
					dark = true
				case ansiWhite:
					dark = false
				case ansiReset:
					dark = false
				default:
					t.Fatalf("line %d: unexpected escape %q", i, seq)
				}
				line = line[end+1:]
				continue
			}
			if !strings.HasPrefix(line, "  ") {
				t.Fatalf("line %d: unexpected drawing %q", i, line[:min(len(line), 4)])
			}
			row = append(row, dark)
			line = line[2:]
		}
		rows = append(rows, row)
	}
	return rows
}

// TestTerminalDrawingMatchesBitmap is the round trip that matters: the drawing
// must map every dark module to a black cell and every light module to a white
// one, at the same position. A polarity flip would still look like a QR code
// but would fail to scan.
func TestTerminalDrawingMatchesBitmap(t *testing.T) {
	const text = "http://192.168.1.5:8080/"
	rendered, err := Terminal(text, gqrcode.Medium)
	if err != nil {
		t.Fatalf("Terminal: %v", err)
	}
	code, err := gqrcode.New(text, gqrcode.Medium)
	if err != nil {
		t.Fatalf("gqrcode.New: %v", err)
	}

	got := parseArt(t, rendered.Art)
	want := code.Bitmap()
	if len(got) != len(want) {
		t.Fatalf("drew %d rows, symbol has %d", len(got), len(want))
	}
	for y := range want {
		if len(got[y]) != len(want[y]) {
			t.Fatalf("row %d has %d modules, symbol has %d", y, len(got[y]), len(want[y]))
		}
		for x := range want[y] {
			if got[y][x] != want[y][x] {
				t.Fatalf("module (%d,%d) = %v, symbol says %v", x, y, got[y][x], want[y][x])
			}
		}
	}
}
