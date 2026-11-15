package diff

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

const patchSample = `--- a.txt
+++ b.txt
@@ -1,2 +1,2 @@
 keep
-old
+new
`

func parsePatch(t *testing.T, args ...string) (*PatchOptions, error) {
	t.Helper()
	root := struct {
		Patch PatchOptions `cmd:"" name:"patch"`
	}{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, err = parser.Parse(append([]string{"patch"}, args...))
	return &root.Patch, err
}

func TestPatchFlagDefaults(t *testing.T) {
	o, err := parsePatch(t)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.File != "" {
		t.Errorf("File = %q, want empty (stdin)", o.File)
	}
	if o.Color != "auto" {
		t.Errorf("Color = %q, want auto", o.Color)
	}
	if o.Theme != "nord" {
		t.Errorf("Theme = %q, want nord", o.Theme)
	}
	if !o.LineNumbers {
		t.Error("LineNumbers should default to true")
	}

	o, err = parsePatch(t, "--no-line-numbers")
	if err != nil {
		t.Fatalf("parse --no-line-numbers: %v", err)
	}
	if o.LineNumbers {
		t.Error("--no-line-numbers left line numbers enabled")
	}
}

func TestPatchColorFlag(t *testing.T) {
	for _, value := range []string{"auto", "always", "never"} {
		if _, err := parsePatch(t, "--color", value); err != nil {
			t.Errorf("--color %s rejected: %v", value, err)
		}
	}
	if _, err := parsePatch(t, "--color", "sometimes"); err == nil {
		t.Fatal("an unknown --color value should be rejected")
	}
}

func TestPatchRendersStdin(t *testing.T) {
	o := &PatchOptions{LineNumbers: true}
	var buf bytes.Buffer
	if err := o.render(strings.NewReader(patchSample), &buf, false); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"1 1 |  keep\n", "2   | -old\n", "  2 | +new\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

func TestPatchRendersFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "change.patch")
	if err := os.WriteFile(path, []byte(patchSample), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	o := &PatchOptions{File: path, LineNumbers: true}
	var buf bytes.Buffer
	if err := o.render(strings.NewReader("ignored"), &buf, false); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(buf.String(), "-old") {
		t.Errorf("file input not rendered:\n%s", buf.String())
	}
}

func TestPatchEmptyInput(t *testing.T) {
	o := &PatchOptions{LineNumbers: true}
	var buf bytes.Buffer
	if err := o.render(strings.NewReader(""), &buf, false); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty input produced %q", buf.String())
	}
}

func TestPatchRejectsNonDiff(t *testing.T) {
	o := &PatchOptions{}
	var buf bytes.Buffer
	err := o.render(strings.NewReader("hello\nworld\n"), &buf, false)
	if err == nil || !strings.Contains(err.Error(), "does not look like a unified diff") {
		t.Fatalf("err = %v, want the not-a-diff error", err)
	}
}

func TestPatchRejectsMalformedPatch(t *testing.T) {
	o := &PatchOptions{}
	var buf bytes.Buffer
	err := o.render(strings.NewReader("--- a\n+++ b\n@@ nope @@\n"), &buf, false)
	if err == nil || !strings.Contains(err.Error(), "parse patch") || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("err = %v, want a parse error with the line number", err)
	}
}

func TestPatchRejectsBinaryInput(t *testing.T) {
	o := &PatchOptions{}
	var buf bytes.Buffer
	err := o.render(strings.NewReader("diff --git a/x b/x\n\x00"), &buf, false)
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("err = %v, want a binary-file error", err)
	}
}

func TestPatchRejectsOversizedInput(t *testing.T) {
	o := &PatchOptions{}
	// A reader that produces the limit plus one byte without holding it.
	var buf bytes.Buffer
	err := o.render(&zerosReader{remaining: maxPatchInputBytes + 1}, &buf, false)
	if err == nil || !strings.Contains(err.Error(), "patch limit") {
		t.Fatalf("err = %v, want a size-limit error", err)
	}
}

func TestPatchColorMode(t *testing.T) {
	o := &PatchOptions{LineNumbers: true, Theme: "default"}
	var colored bytes.Buffer
	if err := o.render(strings.NewReader(patchSample), &colored, true); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(colored.String(), "\x1b[31m-old") {
		t.Errorf("colour mode did not paint the deletion:\n%q", colored.String())
	}
}

// zerosReader returns n bytes without allocating the whole payload, so the
// size-limit test does not need a 50 MiB fixture.
type zerosReader struct{ remaining int64 }

func (r *zerosReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= int64(len(p))
	return len(p), nil
}

func TestPatchDefaultsToNord(t *testing.T) {
	o := &PatchOptions{LineNumbers: true}
	var buf bytes.Buffer
	if err := o.render(strings.NewReader(patchSample), &buf, true); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(buf.String(), "\x1b[38;2;191;97;106m-old") {
		t.Errorf("default rendering is not Nord:\n%q", buf.String())
	}
}
