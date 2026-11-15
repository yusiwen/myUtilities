package patch

import (
	"bytes"
	"strings"
	"testing"
)

func TestNordRenderUsesNordPalette(t *testing.T) {
	files := mustParse(t, basicPatch)
	var buf bytes.Buffer
	if err := Render(&buf, files, Options{Color: true, LineNumbers: true, Theme: ThemeNord}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"\x1b[38;2;191;97;106m-old",       // nord11 deletion
		"\x1b[38;2;163;190;140m+new",      // nord14 addition
		"\x1b[38;2;136;192;208m@@",        // nord8 hunk header
		"\x1b[38;2;191;97;106m--- a.txt",  // nord11 old path
		"\x1b[38;2;163;190;140m+++ b.txt", // nord14 new path
	} {
		if !strings.Contains(out, want) {
			t.Errorf("nord output lacks %q:\n%q", want, out)
		}
	}
	if strings.Contains(out, "\x1b[31m") || strings.Contains(out, "\x1b[32m") {
		t.Errorf("nord output still uses the 16-colour palette:\n%q", out)
	}
}

func TestDefaultThemeKeepsClassicPalette(t *testing.T) {
	files := mustParse(t, basicPatch)
	var buf bytes.Buffer
	if err := Render(&buf, files, Options{Color: true, LineNumbers: true, Theme: ThemeDefault}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\x1b[31m-old") || !strings.Contains(out, "\x1b[32m+new") {
		t.Errorf("default theme changed:\n%q", out)
	}
}

func TestPaint(t *testing.T) {
	if got := Paint(ThemeDefault, LineAdd, "x"); got != "\x1b[32mx\x1b[0m" {
		t.Errorf("default Paint = %q", got)
	}
	if got := Paint(ThemeNord, LineDelete, "x"); got != "\x1b[38;2;191;97;106mx\x1b[0m" {
		t.Errorf("nord Paint = %q", got)
	}
	if got := Paint(ThemeNord, LineContext, ""); got != "" {
		t.Errorf("empty text should stay empty, got %q", got)
	}
}

func TestFGHex(t *testing.T) {
	if got := fgHex(Nord14); got != "\x1b[38;2;163;190;140m" {
		t.Errorf("fgHex(Nord14) = %q", got)
	}
	if got := fgHex("not-a-colour"); got != "" {
		t.Errorf("fgHex(invalid) = %q, want empty", got)
	}
}

func TestThemePaletteMappings(t *testing.T) {
	nord := paletteFor(ThemeNord)
	if nord.add != fgHex(Nord14) || nord.delete != fgHex(Nord11) || nord.hunk != ansiBold+fgHex(Nord8) {
		t.Errorf("nord palette mismatch: %+v", nord)
	}
	if nord.gutter != fgHex(Nord3) {
		t.Errorf("nord gutter = %q, want nord3", nord.gutter)
	}

	def := paletteFor(ThemeDefault)
	if def.add != ansiGreen || def.delete != ansiRed || def.hunk != ansiBold+ansiCyan {
		t.Errorf("default palette mismatch: %+v", def)
	}
}
