package patch

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, input string) []File {
	t.Helper()
	files, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return files
}

func TestParseGitDiff(t *testing.T) {
	input := `diff --git a/hello.txt b/hello.txt
index 1234567..89abcde 100644
--- a/hello.txt
+++ b/hello.txt
@@ -1,3 +1,4 @@ greet
 line one
+line two
 line three
 line four
`
	files := mustParse(t, input)
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "hello.txt" || f.NewPath != "hello.txt" {
		t.Errorf("paths = %q, %q; want hello.txt twice", f.OldPath, f.NewPath)
	}
	if len(f.Header) != 2 || !strings.HasPrefix(f.Header[0], "diff --git ") || !strings.HasPrefix(f.Header[1], "index ") {
		t.Errorf("header = %q", f.Header)
	}
	if f.OldLine != "--- a/hello.txt" || f.NewLine != "+++ b/hello.txt" {
		t.Errorf("raw path lines = %q, %q", f.OldLine, f.NewLine)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(f.Hunks))
	}
	h := f.Hunks[0]
	if h.OldStart != 1 || h.OldCount != 3 || h.NewStart != 1 || h.NewCount != 4 {
		t.Errorf("hunk range = -%d,%d +%d,%d", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
	}
	if !strings.HasSuffix(h.Header, "@@ greet") {
		t.Errorf("hunk header = %q, want the section text kept", h.Header)
	}
	want := []struct {
		kind Kind
		text string
	}{
		{Context, "line one"},
		{Add, "line two"},
		{Context, "line three"},
		{Context, "line four"},
	}
	if len(h.Lines) != len(want) {
		t.Fatalf("lines = %d, want %d", len(h.Lines), len(want))
	}
	for i, w := range want {
		if h.Lines[i].Kind != w.kind || h.Lines[i].Text != w.text {
			t.Errorf("line %d = %+v, want %+v", i, h.Lines[i], w)
		}
	}
}

func TestParseTwoFiles(t *testing.T) {
	input := `diff --git a/a.txt b/a.txt
index 111..222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old a
+new a
diff --git a/b.txt b/b.txt
index 333..444 100644
--- a/b.txt
+++ b/b.txt
@@ -0,0 +1 @@
+only b
`
	files := mustParse(t, input)
	if len(files) != 2 {
		t.Fatalf("files = %d, want 2", len(files))
	}
	if files[0].NewPath != "a.txt" || files[1].NewPath != "b.txt" {
		t.Errorf("paths = %q, %q", files[0].NewPath, files[1].NewPath)
	}
	if len(files[1].Hunks) != 1 || files[1].Hunks[0].OldCount != 0 || files[1].Hunks[0].NewCount != 1 {
		t.Errorf("second hunk = %+v", files[1].Hunks)
	}
}

func TestParsePlainDiffWithTimestamps(t *testing.T) {
	input := "--- old.txt\t2026-11-14 10:00:00.000000000 +0800\n" +
		"+++ new.txt\t2026-11-14 10:00:01.000000000 +0800\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"
	files := mustParse(t, input)
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if f.OldPath != "old.txt" || f.NewPath != "new.txt" {
		t.Errorf("paths = %q, %q; timestamps should be stripped", f.OldPath, f.NewPath)
	}
	if f.OldLine != "--- old.txt\t2026-11-14 10:00:00.000000000 +0800" {
		t.Errorf("raw old line = %q", f.OldLine)
	}
}

func TestParseNoNewlineMarkers(t *testing.T) {
	input := `--- a.txt
+++ b.txt
@@ -1 +1 @@
-old
\ No newline at end of file
+new
\ No newline at end of file
`
	files := mustParse(t, input)
	h := files[0].Hunks[0]
	if len(h.Lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(h.Lines))
	}
	if h.Lines[0].Kind != Delete || h.Lines[1].Kind != NoNewline || h.Lines[2].Kind != Add || h.Lines[3].Kind != NoNewline {
		t.Errorf("kinds = %v", h.Lines)
	}
}

func TestParseBinaryNotice(t *testing.T) {
	input := `diff --git a/logo.png b/logo.png
index aaa..bbb 100644
Binary files a/logo.png and b/logo.png differ
`
	files := mustParse(t, input)
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}
	f := files[0]
	if !f.Binary {
		t.Error("Binary = false, want true")
	}
	if f.OldPath != "logo.png" || f.NewPath != "logo.png" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	if len(f.Hunks) != 0 {
		t.Errorf("hunks = %d, want none", len(f.Hunks))
	}
}

func TestParseRenameMetadata(t *testing.T) {
	input := `diff --git a/old.txt b/new.txt
similarity index 90%
rename from old.txt
rename to new.txt
`
	files := mustParse(t, input)
	f := files[0]
	if f.OldPath != "old.txt" || f.NewPath != "new.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
	joined := strings.Join(f.Header, "\n")
	if !strings.Contains(joined, "rename from old.txt") || !strings.Contains(joined, "similarity index 90%") {
		t.Errorf("header = %q", f.Header)
	}
}

func TestParseQuotedPaths(t *testing.T) {
	input := `diff --git "a/my file.txt" "b/my file.txt"
index 111..222 100644
--- "a/my file.txt"
+++ "b/my file.txt"
@@ -1 +1 @@
-a
+b
`
	files := mustParse(t, input)
	f := files[0]
	if f.OldPath != "my file.txt" || f.NewPath != "my file.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
}

func TestParseCRLF(t *testing.T) {
	input := "--- a.txt\r\n+++ b.txt\r\n@@ -1 +1 @@\r\n-old\r\n+new\r\n"
	files := mustParse(t, input)
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("files = %+v", files)
	}
	h := files[0].Hunks[0]
	if h.Lines[0].Text != "old" || h.Lines[1].Text != "new" {
		t.Errorf("texts = %q, %q; CR should be stripped", h.Lines[0].Text, h.Lines[1].Text)
	}
}

func TestParseFormatPatchPreamble(t *testing.T) {
	input := `From 1234567890 Mon Sep 17 00:00:00 2001
From: Someone <someone@example.com>
Subject: [PATCH] touch a file

---
 a.txt | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)

diff --git a/a.txt b/a.txt
index 111..222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-a
+b
-- 
2.40.0
`
	files := mustParse(t, input)
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1; the mail preamble must be ignored", len(files))
	}
	if files[0].NewPath != "a.txt" {
		t.Errorf("path = %q", files[0].NewPath)
	}
}

func TestParseEmptyAndGarbage(t *testing.T) {
	files, err := Parse(nil)
	if err != nil || len(files) != 0 {
		t.Fatalf("Parse(nil) = %v, %v", files, err)
	}

	files, err = Parse([]byte("just some text\nwith no diff at all\n"))
	if err != nil {
		t.Fatalf("garbage returned an error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("garbage produced %d files", len(files))
	}
}

func TestParseRejectsBinaryInput(t *testing.T) {
	if _, err := Parse([]byte("diff --git a/x b/x\n\x00")); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("err = %v, want a binary-file error", err)
	}
}

func TestParseRejectsMalformedHunks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "missing body lines",
			input: "--- a\n+++ b\n@@ -1,3 +1,3 @@\n only one\n",
			want:  "ended early",
		},
		{
			name:  "invalid header",
			input: "--- a\n+++ b\n@@ nope @@\n",
			want:  "invalid hunk header",
		},
		{
			name:  "combined diff hunk",
			input: "--- a\n+++ b\n@@@ -1,2 -1,2 +1,3 @@@\n",
			want:  "combined diffs",
		},
		{
			name:  "combined diff header",
			input: "diff --cc a.txt\n",
			want:  "combined diffs",
		},
		{
			name:  "hunk before file",
			input: "@@ -1 +1 @@\n-a\n+b\n",
			want:  "hunk without a file header",
		},
		{
			name:  "unexpected body line",
			input: "--- a\n+++ b\n@@ -1 +1 @@\n?what\n",
			want:  "unexpected line in hunk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "line ") {
				t.Errorf("err = %v, want the offending line number", err)
			}
		})
	}
}
