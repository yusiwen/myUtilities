package patch

import (
	"bytes"
	"strings"
	"testing"
)

func renderString(t *testing.T, input string, opts Options) string {
	t.Helper()
	files := mustParse(t, input)
	var buf bytes.Buffer
	if err := Render(&buf, files, opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

const basicPatch = `--- a.txt
+++ b.txt
@@ -1,2 +1,2 @@
 keep
-old
+new
`

func TestRenderWithLineNumbers(t *testing.T) {
	got := renderString(t, basicPatch, Options{LineNumbers: true})
	want := `--- a.txt
+++ b.txt
@@ -1,2 +1,2 @@
1 1 |  keep
2   | -old
  2 | +new
`
	if got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderWithoutLineNumbers(t *testing.T) {
	got := renderString(t, basicPatch, Options{})
	want := `--- a.txt
+++ b.txt
@@ -1,2 +1,2 @@
 keep
-old
+new
`
	if got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderGutterWidthFollowsLineNumbers(t *testing.T) {
	input := `--- a.txt
+++ b.txt
@@ -99,2 +99,2 @@
 keep
-old
+new
`
	got := renderString(t, input, Options{LineNumbers: true})
	if !strings.Contains(got, " 99  99 |  keep") {
		t.Errorf("gutter did not size for three-digit numbers:\n%s", got)
	}
}

func TestRenderMultipleFilesSeparated(t *testing.T) {
	input := `--- a.txt
+++ a.txt
@@ -1 +1 @@
-old a
+new a
--- b.txt
+++ b.txt
@@ -1 +1 @@
-old b
+new b
`
	got := renderString(t, input, Options{LineNumbers: true})
	if !strings.Contains(got, "+new a\n\n--- b.txt\n") {
		t.Errorf("files are not separated by a blank line:\n%s", got)
	}
}

func TestRenderNoNewlineMarker(t *testing.T) {
	input := "--- a.txt\n+++ b.txt\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n\\ No newline at end of file\n"
	got := renderString(t, input, Options{LineNumbers: true})
	if !strings.Contains(got, "\\ No newline at end of file") {
		t.Fatalf("marker missing:\n%s", got)
	}
	if strings.Count(got, `\ No newline at end of file`) != 2 {
		t.Errorf("want two no-newline markers:\n%s", got)
	}
}

func TestRenderBinaryNotice(t *testing.T) {
	input := `diff --git a/logo.png b/logo.png
index aaa..bbb 100644
Binary files a/logo.png and b/logo.png differ
`
	got := renderString(t, input, Options{LineNumbers: true})
	for _, want := range []string{"diff --git a/logo.png b/logo.png", "index aaa..bbb 100644", "Binary files a/logo.png and b/logo.png differ"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "@@") {
		t.Errorf("binary output should have no hunk header:\n%s", got)
	}
}

func TestRenderColorAddsANSI(t *testing.T) {
	colored := renderString(t, basicPatch, Options{Color: true, LineNumbers: true})
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("coloured output has no ANSI escapes: %q", colored)
	}
	if !strings.Contains(colored, "\x1b[32m+new") {
		t.Errorf("addition is not green: %q", colored)
	}
	if !strings.Contains(colored, "\x1b[31m-old") {
		t.Errorf("deletion is not red: %q", colored)
	}

	plain := renderString(t, basicPatch, Options{LineNumbers: true})
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("plain output contains ANSI escapes: %q", plain)
	}
}

func TestRenderStripsNothingFromContent(t *testing.T) {
	// Tabs, trailing spaces and unicode must survive rendering.
	input := "--- a.txt\n+++ b.txt\n@@ -1 +1 @@\n-\told  \n+\t新的  \n"
	got := renderString(t, input, Options{})
	if !strings.Contains(got, "-\told  \n") || !strings.Contains(got, "+\t新的  \n") {
		t.Errorf("content changed:\n%q", got)
	}
}
