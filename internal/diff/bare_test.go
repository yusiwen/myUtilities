package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/patch"
)

// parseDiffCLI parses the real `mu diff` command tree, including subcommands.
func parseDiffCLI(t *testing.T, args ...string) (*Options, *kong.Context, error) {
	t.Helper()
	root := struct {
		Diff Options `cmd:"" name:"diff"`
	}{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	ctx, err := parser.Parse(append([]string{"diff"}, args...))
	return &root.Diff, ctx, err
}

func TestBareKongRouting(t *testing.T) {
	o, ctx, err := parseDiffCLI(t)
	if err != nil {
		t.Fatalf("parse bare: %v", err)
	}
	if got := ctx.Selected().Name; got != "bare" {
		t.Errorf("selected = %q, want bare", got)
	}
	if o.Bare.Color != "auto" || !o.Bare.LineNumbers || o.Bare.Theme != "nord" || o.Bare.TUI {
		t.Errorf("defaults = %+v", o.Bare)
	}

	o, _, err = parseDiffCLI(t, "a.txt", "b.txt")
	if err != nil {
		t.Fatalf("parse two files: %v", err)
	}
	if len(o.Bare.Files) != 2 || o.Bare.Files[0] != "a.txt" || o.Bare.Files[1] != "b.txt" {
		t.Errorf("Files = %v", o.Bare.Files)
	}

	o, _, err = parseDiffCLI(t, "--tui", "--theme", "default", "--no-line-numbers", "a.txt", "b.txt")
	if err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if !o.Bare.TUI || o.Bare.Theme != "default" || o.Bare.LineNumbers {
		t.Errorf("flags = %+v", o.Bare)
	}

	// Subcommands must keep winning over the default command.
	o, ctx, err = parseDiffCLI(t, "file", "a.txt", "b.txt")
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}
	if ctx.Selected().Name != "file" || o.File.File1 != "a.txt" || o.File.File2 != "b.txt" {
		t.Errorf("selected = %q, file = %+v", ctx.Selected().Name, o.File)
	}
	if len(o.Bare.Files) != 0 {
		t.Errorf("bare command also consumed args: %v", o.Bare.Files)
	}

	if _, ctx, err = parseDiffCLI(t, "patch", "--color", "never"); err != nil || ctx.Selected().Name != "patch" {
		t.Errorf("patch subcommand: selected=%v err=%v", ctx, err)
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestCompareFilesModified(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeFile(t, dir, "old.txt", "a\nb\nc\n")
	newPath := writeFile(t, dir, "new.txt", "a\nB\nc\n")

	files, err := compareFiles(oldPath, newPath)
	if err != nil {
		t.Fatalf("compareFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != oldPath || f.NewPath != newPath {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if len(f.Hunks) != 1 || f.Hunks[0].Header != "@@ -1,3 +1,3 @@" {
		t.Fatalf("hunks = %+v", f.Hunks)
	}
	var buf strings.Builder
	if err := patch.Render(&buf, files, patch.Options{LineNumbers: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"2   | -b\n", "  2 | +B\n"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("output missing %q:\n%s", want, buf.String())
		}
	}
}

func TestCompareFilesIdentical(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "same\n")
	b := writeFile(t, dir, "b.txt", "same\n")
	files, err := compareFiles(a, b)
	if err != nil {
		t.Fatalf("compareFiles: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("identical files produced %d file(s)", len(files))
	}
}

func TestCompareFilesAddedFromNullDevice(t *testing.T) {
	dir := t.TempDir()
	newPath := writeFile(t, dir, "new.txt", "brand new\n")
	files, err := compareFiles("/dev/null", newPath)
	if err != nil {
		t.Fatalf("compareFiles: %v", err)
	}
	if len(files) != 1 || files[0].OldPath != "/dev/null" {
		t.Fatalf("files = %+v", files)
	}
	if got := statsFor(files[0]).status; got != 'A' {
		t.Errorf("status = %q, want A", got)
	}
}

func TestReadDiffInputRejectsDirectory(t *testing.T) {
	if _, err := readDiffInput(t.TempDir()); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadDiffInputMissingFile(t *testing.T) {
	if _, err := readDiffInput(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("a missing file should be an error")
	}
}

func TestBareRejectsTooManyArguments(t *testing.T) {
	o := &BareOptions{Files: []string{"a", "b", "c"}}
	err := o.Run()
	if err == nil || !strings.Contains(err.Error(), "got 3 arguments") {
		t.Fatalf("err = %v", err)
	}
}
