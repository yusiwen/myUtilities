package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func parseMerge(t *testing.T, args ...string) (*MergeOptions, error) {
	t.Helper()
	root := struct {
		Merge MergeOptions `cmd:"" name:"merge"`
	}{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, err = parser.Parse(append([]string{"merge"}, args...))
	return &root.Merge, err
}

func TestMergeFlagDefaults(t *testing.T) {
	o, err := parseMerge(t, "--base", "b.txt", "--ours", "o.txt", "--theirs", "t.txt")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.Style != "merge" {
		t.Errorf("Style = %q, want merge", o.Style)
	}
	if o.MarkerSize != 7 {
		t.Errorf("MarkerSize = %d, want 7", o.MarkerSize)
	}
	if o.Check {
		t.Error("Check should default to false")
	}
}

func TestMergeRequiresThreeInputs(t *testing.T) {
	tests := [][]string{
		{},
		{"--base", "b.txt", "--ours", "o.txt"},
		{"--base", "b.txt", "--theirs", "t.txt"},
		{"--ours", "o.txt", "--theirs", "t.txt"},
	}
	for _, args := range tests {
		if _, err := parseMerge(t, args...); err == nil {
			t.Errorf("parse %v should fail without all three inputs", args)
		}
	}
}

func TestMergeRejectsUnknownStyle(t *testing.T) {
	if _, err := parseMerge(t, "--base", "b", "--ours", "o", "--theirs", "t", "--style", "wheel"); err == nil {
		t.Fatal("an unknown --style should be rejected")
	}
	if _, err := conflictStyle("wheel"); err == nil {
		t.Fatal("conflictStyle should reject an unknown style")
	}
}

func writeInput(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestMergeWithFiles(t *testing.T) {
	dir := t.TempDir()
	base := writeInput(t, dir, "base.txt", "A\nB\nC\n")
	ours := writeInput(t, dir, "ours.txt", "A\nX\nC\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\nY\nC\n")

	o := &MergeOptions{Base: base, Ours: ours, Theirs: theirs, Style: "diff3"}
	res, err := o.mergeWith(strings.NewReader(""))
	if err != nil {
		t.Fatalf("mergeWith: %v", err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", res.Conflicts)
	}
	text := string(res.Text)
	for _, want := range []string{"<<<<<<< ours.txt\n", "||||||| base.txt\n", ">>>>>>> theirs.txt\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Error("merge output must be plain text without ANSI escapes")
	}
}

func TestMergeCleanIsExact(t *testing.T) {
	dir := t.TempDir()
	base := writeInput(t, dir, "base.txt", "A\nB\n")
	ours := writeInput(t, dir, "ours.txt", "A\nB\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\nZ\n")

	o := &MergeOptions{Base: base, Ours: ours, Theirs: theirs, Style: "merge"}
	res, err := o.mergeWith(strings.NewReader(""))
	if err != nil {
		t.Fatalf("mergeWith: %v", err)
	}
	if res.Conflicts != 0 {
		t.Fatalf("Conflicts = %d, want 0", res.Conflicts)
	}
	if string(res.Text) != "A\nZ\n" {
		t.Errorf("Text = %q, want %q", res.Text, "A\nZ\n")
	}
}

func TestMergeCustomLabelsAndMarkerSize(t *testing.T) {
	dir := t.TempDir()
	base := writeInput(t, dir, "base.txt", "A\nB\n")
	ours := writeInput(t, dir, "ours.txt", "A\nX\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\nY\n")

	o := &MergeOptions{
		Base: base, Ours: ours, Theirs: theirs,
		Style: "diff3", OursLabel: "local", BaseLabel: "ancestor", TheirsLabel: "remote",
		MarkerSize: 4,
	}
	res, err := o.mergeWith(strings.NewReader(""))
	if err != nil {
		t.Fatalf("mergeWith: %v", err)
	}
	text := string(res.Text)
	for _, want := range []string{"<<<< local\n", "|||| ancestor\n", ">>>> remote\n", "====\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

func TestMergeReadsOneInputFromStdin(t *testing.T) {
	dir := t.TempDir()
	ours := writeInput(t, dir, "ours.txt", "A\nX\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\nY\n")

	o := &MergeOptions{Base: "-", Ours: ours, Theirs: theirs}
	res, err := o.mergeWith(strings.NewReader("A\nB\n"))
	if err != nil {
		t.Fatalf("mergeWith: %v", err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1", res.Conflicts)
	}
	if !strings.Contains(string(res.Text), "<<<<<<< ours.txt\n") {
		t.Errorf("stdin label should stay the role name for stdin only:\n%s", res.Text)
	}

	o = &MergeOptions{Base: "-", Ours: "-", Theirs: theirs}
	if _, err := o.mergeWith(strings.NewReader("")); err == nil {
		t.Fatal("two stdin inputs should be rejected")
	}
}

func TestMergeRejectsBinaryInput(t *testing.T) {
	dir := t.TempDir()
	base := writeInput(t, dir, "base.bin", "A\x00B")
	ours := writeInput(t, dir, "ours.txt", "A\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\n")

	o := &MergeOptions{Base: base, Ours: ours, Theirs: theirs}
	if _, err := o.mergeWith(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("err = %v, want a binary-file error", err)
	}
}

func TestMergeRejectsOversizedInput(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.txt")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := f.Truncate(maxMergeInputBytes + 1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ours := writeInput(t, dir, "ours.txt", "A\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\n")

	o := &MergeOptions{Base: big, Ours: ours, Theirs: theirs}
	if _, err := o.mergeWith(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "merge limit") {
		t.Fatalf("err = %v, want a size-limit error", err)
	}
}

func TestMergeRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	ours := writeInput(t, dir, "ours.txt", "A\n")
	theirs := writeInput(t, dir, "theirs.txt", "A\n")

	o := &MergeOptions{Base: filepath.Join(dir, "missing.txt"), Ours: ours, Theirs: theirs}
	if _, err := o.mergeWith(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("err = %v, want the missing path", err)
	}
}
