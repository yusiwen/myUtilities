package diff

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/patch"
)

func TestDiffTextTheme(t *testing.T) {
	def := diffText("old", "new", patch.ThemeDefault)
	if !strings.Contains(def, "\x1b[31mold") || !strings.Contains(def, "\x1b[32mnew") {
		t.Errorf("default diffText changed: %q", def)
	}

	nord := diffText("old", "new", patch.ThemeNord)
	for _, want := range []string{"\x1b[38;2;191;97;106mold", "\x1b[38;2;163;190;140mnew"} {
		if !strings.Contains(nord, want) {
			t.Errorf("nord diffText lacks %q: %q", want, nord)
		}
	}
	if strings.Contains(nord, "\x1b[31m") || strings.Contains(nord, "\x1b[32m") {
		t.Errorf("nord diffText still uses the 16-colour palette: %q", nord)
	}
}

func TestPatchThemeMapping(t *testing.T) {
	if patchTheme("nord") != patch.ThemeNord {
		t.Error("patchTheme(nord) did not select the Nord theme")
	}
	if patchTheme("default") != patch.ThemeDefault {
		t.Error("patchTheme(default) did not select the default theme")
	}
	if patchTheme("") != patch.ThemeNord {
		t.Error("an empty theme should resolve to Nord")
	}
}

func TestFileAndTextThemeFlags(t *testing.T) {
	root := struct {
		File FileOptions `cmd:"" name:"file"`
		Text TextOptions `cmd:"" name:"text"`
	}{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}

	fileArgs := []string{"file", "a", "b"}
	if _, err := parser.Parse(fileArgs); err != nil {
		t.Fatalf("parse file: %v", err)
	}
	if root.File.Theme != "nord" {
		t.Errorf("file Theme = %q, want nord", root.File.Theme)
	}

	if _, err := parser.Parse([]string{"file", "a", "b", "--theme", "default"}); err != nil {
		t.Fatalf("parse file --theme default: %v", err)
	}
	if root.File.Theme != "default" {
		t.Errorf("file Theme = %q, want default", root.File.Theme)
	}

	if _, err := parser.Parse([]string{"text", "a", "b", "--theme", "default"}); err != nil {
		t.Fatalf("parse text --theme default: %v", err)
	}
	if root.Text.Theme != "default" {
		t.Errorf("text Theme = %q, want default", root.Text.Theme)
	}

	if _, err := parser.Parse([]string{"file", "a", "b", "--theme", "solarized"}); err == nil {
		t.Error("an unknown theme should be rejected")
	}
}

func TestPatchThemeFlag(t *testing.T) {
	o, err := parsePatch(t)
	if err != nil {
		t.Fatalf("parse default: %v", err)
	}
	if o.Theme != "nord" {
		t.Errorf("default Theme = %q, want nord", o.Theme)
	}

	o, err = parsePatch(t, "--theme", "nord")
	if err != nil {
		t.Fatalf("parse --theme nord: %v", err)
	}
	if o.Theme != "nord" {
		t.Errorf("Theme = %q, want nord", o.Theme)
	}

	if _, err := parsePatch(t, "--theme", "dracula"); err == nil {
		t.Error("an unknown patch theme should be rejected")
	}
}
