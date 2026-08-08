package scip

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeIndexer(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "fake-indexer")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunIndexerSuccessRemovesTemp(t *testing.T) {
	bin := writeFakeIndexer(t, `printf "some noise\n" >&2
touch "$1"
`)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.scip")
	tc := &Toolchain{Out: io.Discard}

	res := runIndexer(tc, bin, []string{outPath}, dir, outPath)
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.LogPath != "" {
		t.Fatalf("expected no retained temp file on success, got %q", res.LogPath)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
}

func TestRunIndexerFailureRetainsLog(t *testing.T) {
	bin := writeFakeIndexer(t, `printf "BUILD OK noise line\n" >&2
printf "ERROR: something broke\n" >&2
printf "Caused by: boom\n" >&2
exit 1
`)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.scip")
	tc := &Toolchain{Out: io.Discard}

	res := runIndexer(tc, bin, []string{outPath}, dir, outPath)
	if res.Success {
		t.Fatal("expected failure")
	}
	if res.LogPath == "" {
		t.Fatal("expected a retained temp log path on failure")
	}
	defer os.Remove(res.LogPath)

	data, err := os.ReadFile(res.LogPath)
	if err != nil {
		t.Fatalf("retained log missing: %v", err)
	}
	if !strings.Contains(string(data), "ERROR: something broke") {
		t.Fatalf("retained log missing error line: %q", string(data))
	}
	if !strings.Contains(res.Log, "Caused by: boom") {
		t.Fatalf("expected parsed log to include Caused by line, got %q", res.Log)
	}
}

func TestRunIndexerVerboseStreamsNoTemp(t *testing.T) {
	bin := writeFakeIndexer(t, `printf "live noise\n" >&2
touch "$1"
`)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.scip")
	var buf bytes.Buffer
	tc := &Toolchain{Out: &buf, Verbose: true}

	res := runIndexer(tc, bin, []string{outPath}, dir, outPath)
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if res.LogPath != "" {
		t.Fatalf("verbose mode must not create a temp file, got %q", res.LogPath)
	}
	if !strings.Contains(buf.String(), "live noise") {
		t.Fatalf("verbose mode should stream indexer output, got %q", buf.String())
	}
}

func TestExtractError(t *testing.T) {
	cases := []struct {
		name, log, wantSub string
	}{
		{"picks error keywords", "noise\n[ERROR] build failed\nmore\n", "[ERROR] build failed"},
		{"picks caused by", "noise\nCaused by: NullPointerException\n", "Caused by"},
		{"filters JNA warnings", "WARNING: restricted method\nBUILD FAILURE\n", "BUILD FAILURE"},
		{"falls back to tail", "line1\nline2\nline3\n", "line3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractError(c.log)
			if !strings.Contains(got, c.wantSub) {
				t.Fatalf("extractError(%q) = %q, want substring %q", c.log, got, c.wantSub)
			}
		})
	}
	if got := extractError(""); got != "" {
		t.Fatalf("expected empty log to produce empty result, got %q", got)
	}
}

func TestGenerateFailureMessage(t *testing.T) {
	bin := writeFakeIndexer(t, `printf "ERROR: failed to build\n" >&2
exit 1
`)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.scip")
	tc := &Toolchain{Out: io.Discard}
	ix := &Indexer{BinaryName: "scip-java", Lang: "java"}

	err := generate(tc, ix, bin, dir, outPath, "")
	if err == nil {
		t.Fatal("expected generate to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "scip-java indexer failed") {
		t.Fatalf("expected indexer name in error, got %q", msg)
	}
	if !strings.Contains(msg, "ERROR: failed to build") {
		t.Fatalf("expected extracted error in message, got %q", msg)
	}
	if !strings.Contains(msg, "Full indexer log kept at:") {
		t.Fatalf("expected retained log path hint in message, got %q", msg)
	}
	// generate() retains the log file for the user by design; the test must
	// remove it.
	const marker = "Full indexer log kept at: "
	if i := strings.Index(msg, marker); i >= 0 {
		os.Remove(strings.TrimSpace(msg[i+len(marker):]))
	}
}

func TestBuildArgs(t *testing.T) {
	goIX, _ := LookupLang("go")
	javaIX, _ := LookupLang("java")
	rustIX, _ := LookupLang("rust")

	quiet := &Toolchain{}
	verbose := &Toolchain{Verbose: true}

	// Go: unchanged from the original hardcoded invocation.
	got := buildArgs(quiet, goIX, "/cache/out.scip")
	if strings.Join(got, " ") != "-o /cache/out.scip -q" {
		t.Fatalf("go args = %v", got)
	}
	if got := buildArgs(verbose, goIX, "/cache/out.scip"); strings.Join(got, " ") != "-o /cache/out.scip" {
		t.Fatalf("go verbose args = %v", got)
	}

	// Java: index subcommand with --output; no quiet flag.
	got = buildArgs(quiet, javaIX, "/cache/out.scip")
	if strings.Join(got, " ") != "index --output /cache/out.scip" {
		t.Fatalf("java args = %v", got)
	}
	if got := buildArgs(verbose, javaIX, "/cache/out.scip"); strings.Join(got, " ") != "index --output /cache/out.scip" {
		t.Fatalf("java verbose args = %v", got)
	}

	// Rust: `rust-analyzer scip --output <path> .` (cwd = repo root); no quiet flag.
	got = buildArgs(quiet, rustIX, "/cache/out.scip")
	if strings.Join(got, " ") != "scip --output /cache/out.scip ." {
		t.Fatalf("rust args = %v", got)
	}
	if got := buildArgs(verbose, rustIX, "/cache/out.scip"); strings.Join(got, " ") != "scip --output /cache/out.scip ." {
		t.Fatalf("rust verbose args = %v", got)
	}
}

func TestIndexErrorHard(t *testing.T) {
	javaIX, _ := LookupLang("java")
	goIX, _ := LookupLang("go")

	e := indexErr(javaIX, errors.New("boom"))
	var hard *IndexError
	if !errors.As(e, &hard) || !hard.Hard {
		t.Fatalf("expected java IndexError to be hard, got %+v", e)
	}
	e = indexErr(goIX, errors.New("boom"))
	if errors.As(e, &hard) && hard.Hard {
		t.Fatalf("expected go IndexError to be non-hard, got %+v", e)
	}
}
