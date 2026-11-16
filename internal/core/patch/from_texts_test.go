package patch

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func singleFile(t *testing.T, files []File) File {
	t.Helper()
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	return files[0]
}

func TestFromTextsIdentical(t *testing.T) {
	if files := FromTexts("a", "b", []byte("same\n"), []byte("same\n")); files != nil {
		t.Fatalf("identical texts produced %d file(s)", len(files))
	}
	if files := FromTexts("a", "b", nil, nil); files != nil {
		t.Fatalf("empty texts produced %d file(s)", len(files))
	}
}

func TestFromTextsModifyMiddleLine(t *testing.T) {
	f := singleFile(t, FromTexts("old.txt", "new.txt", []byte("a\nb\nc\n"), []byte("a\nB\nc\n")))

	if f.OldPath != "old.txt" || f.NewPath != "new.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if f.OldLine != "--- old.txt" || f.NewLine != "+++ new.txt" {
		t.Errorf("headers = %q, %q", f.OldLine, f.NewLine)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(f.Hunks))
	}
	h := f.Hunks[0]
	if h.Header != "@@ -1,3 +1,3 @@" || h.OldStart != 1 || h.OldCount != 3 || h.NewStart != 1 || h.NewCount != 3 {
		t.Errorf("hunk = %q (%d,%d %d,%d)", h.Header, h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	want := []Line{
		{Kind: Context, Text: "a"},
		{Kind: Delete, Text: "b"},
		{Kind: Add, Text: "B"},
		{Kind: Context, Text: "c"},
	}
	if len(h.Lines) != len(want) {
		t.Fatalf("lines = %v", h.Lines)
	}
	for i := range want {
		if h.Lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, h.Lines[i], want[i])
		}
	}
}

func TestFromTextsAddToEmptyFile(t *testing.T) {
	f := singleFile(t, FromTexts("/dev/null", "new.txt", nil, []byte("x\ny\n")))
	h := f.Hunks[0]
	if h.Header != "@@ -0,0 +1,2 @@" || h.OldStart != 0 || h.OldCount != 0 || h.NewStart != 1 || h.NewCount != 2 {
		t.Errorf("hunk = %q (%d,%d %d,%d)", h.Header, h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	if len(h.Lines) != 2 || h.Lines[0].Kind != Add || h.Lines[1].Kind != Add {
		t.Errorf("lines = %v", h.Lines)
	}
}

func TestFromTextsDeleteAllLines(t *testing.T) {
	f := singleFile(t, FromTexts("old.txt", "/dev/null", []byte("a\nb\n"), nil))
	h := f.Hunks[0]
	if h.Header != "@@ -1,2 +0,0 @@" || h.NewStart != 0 || h.NewCount != 0 {
		t.Errorf("hunk = %q (%d,%d %d,%d)", h.Header, h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	if len(h.Lines) != 2 || h.Lines[0].Kind != Delete || h.Lines[1].Kind != Delete {
		t.Errorf("lines = %v", h.Lines)
	}
}

func TestFromTextsInsertAtStart(t *testing.T) {
	f := singleFile(t, FromTexts("old.txt", "new.txt", []byte("b\nc\n"), []byte("a\nb\nc\n")))
	h := f.Hunks[0]
	if h.Header != "@@ -1,2 +1,3 @@" || h.OldStart != 1 || h.OldCount != 2 || h.NewStart != 1 || h.NewCount != 3 {
		t.Errorf("hunk = %q (%d,%d %d,%d)", h.Header, h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	if len(h.Lines) != 3 || h.Lines[0].Kind != Add || h.Lines[1].Kind != Context || h.Lines[2].Kind != Context {
		t.Errorf("lines = %v", h.Lines)
	}
}

func numberedFile(t *testing.T, changes map[int]string) []byte {
	t.Helper()
	var sb strings.Builder
	for i := 1; i <= 30; i++ {
		if repl, ok := changes[i]; ok {
			fmt.Fprintf(&sb, "%s\n", repl)
			continue
		}
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return []byte(sb.String())
}

func TestFromTextsHunkSplittingMatchesDiffU3(t *testing.T) {
	base := numberedFile(t, nil)

	// A six-line unchanged gap merges into one hunk.
	merged := singleFile(t, FromTexts("a", "b", base, numberedFile(t, map[int]string{5: "X", 12: "Y"})))
	if len(merged.Hunks) != 1 {
		t.Fatalf("gap 6 produced %d hunks, want 1", len(merged.Hunks))
	}
	if merged.Hunks[0].Header != "@@ -2,14 +2,14 @@" {
		t.Errorf("merged header = %q", merged.Hunks[0].Header)
	}

	// A seven-line gap stays two hunks.
	split := singleFile(t, FromTexts("a", "b", base, numberedFile(t, map[int]string{5: "X", 13: "Y"})))
	if len(split.Hunks) != 2 {
		t.Fatalf("gap 7 produced %d hunks, want 2", len(split.Hunks))
	}
	if got := split.Hunks[0].Header; got != "@@ -2,7 +2,7 @@" {
		t.Errorf("first header = %q", got)
	}
	if got := split.Hunks[1].Header; got != "@@ -10,7 +10,7 @@" {
		t.Errorf("second header = %q", got)
	}
}

func TestFromTextsMissingFinalNewline(t *testing.T) {
	f := singleFile(t, FromTexts("a", "b", []byte("a\nb"), []byte("a\nB")))
	h := f.Hunks[0]
	want := []Kind{Context, Delete, NoNewline, Add, NoNewline}
	if len(h.Lines) != len(want) {
		t.Fatalf("lines = %v", h.Lines)
	}
	for i, kind := range want {
		if h.Lines[i].Kind != kind {
			t.Errorf("line %d kind = %v, want %v", i, h.Lines[i].Kind, kind)
		}
	}
}

func TestFromTextsOnlyFinalNewlineDiffers(t *testing.T) {
	f := singleFile(t, FromTexts("a", "b", []byte("a"), []byte("a\n")))
	h := f.Hunks[0]
	want := []Kind{Delete, NoNewline, Add}
	if len(h.Lines) != len(want) {
		t.Fatalf("lines = %v", h.Lines)
	}
	for i, kind := range want {
		if h.Lines[i].Kind != kind {
			t.Errorf("line %d kind = %v, want %v", i, h.Lines[i].Kind, kind)
		}
	}
}

func TestFromTextsCRLF(t *testing.T) {
	f := singleFile(t, FromTexts("a", "b", []byte("a\r\nb\r\n"), []byte("a\r\nB\r\n")))
	h := f.Hunks[0]
	if len(h.Lines) != 3 {
		t.Fatalf("lines = %v", h.Lines)
	}
	for _, line := range h.Lines {
		if strings.ContainsRune(line.Text, '\r') || strings.ContainsRune(line.Text, '\n') {
			t.Errorf("line %q kept its terminator", line.Text)
		}
	}
	if h.Lines[1].Text != "b" || h.Lines[2].Text != "B" {
		t.Errorf("content = %q, %q", h.Lines[1].Text, h.Lines[2].Text)
	}
}

func TestFromTextsBinary(t *testing.T) {
	f := singleFile(t, FromTexts("a.bin", "b.bin", []byte("a\x00"), []byte("b\x00")))
	if !f.Binary || len(f.Hunks) != 0 {
		t.Errorf("Binary = %v, hunks = %d", f.Binary, len(f.Hunks))
	}
	if len(f.Header) != 1 || f.Header[0] != "Binary files a.bin and b.bin differ" {
		t.Errorf("header = %v", f.Header)
	}
}

func TestFromTextsUnicode(t *testing.T) {
	f := singleFile(t, FromTexts("a", "b", []byte("第一行\n第二行\n第三行\n"), []byte("第一行\n變更行\n第三行\n")))
	h := f.Hunks[0]
	if h.Lines[1].Text != "第二行" || h.Lines[2].Text != "變更行" {
		t.Errorf("lines = %v", h.Lines)
	}
}

// TestFromTextsRender pins the shared renderer path: a generated diff renders
// with the same gutter and marker layout as a parsed patch.
func TestFromTextsRender(t *testing.T) {
	files := FromTexts("old.txt", "new.txt", []byte("a\nb\nc\n"), []byte("a\nB\nc\n"))
	var buf bytes.Buffer
	if err := Render(&buf, files, Options{LineNumbers: true}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"--- old.txt\n", "+++ new.txt\n", "@@ -1,3 +1,3 @@\n", "1 1 |  a\n", "2   | -b\n", "  2 | +B\n", "3 3 |  c\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}
