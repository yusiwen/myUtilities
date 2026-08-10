package runner

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// These tests exercise the non-TTY path (test stdout is never a terminal),
// which is the branch that executes in piped/scripted usage.
func TestCommandRunnerSuccess(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "greet", CmdLine: "echo hello world"},
		{Name: "count", CmdLine: "printf 'a\nb\nc\n'"},
	})
	if err := r.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCommandRunnerFailure(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "ok", CmdLine: "echo fine"},
		{Name: "boom", CmdLine: "echo oops 1>&2; exit 3"},
		{Name: "never", CmdLine: "echo should not run"},
	})
	err := r.Run()
	if err == nil {
		t.Fatal("expected error from failing command")
	}
	if !strings.Contains(err.Error(), "exit code 3") {
		t.Fatalf("error should mention exit code 3, got: %v", err)
	}
	if !strings.Contains(err.Error(), "oops") {
		t.Fatalf("error should include stderr output, got: %v", err)
	}
}

func TestCommandRunnerEmptyStderr(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "silent", CmdLine: "exit 1"},
	})
	err := r.Run()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no error output") {
		t.Fatalf("expected fallback message, got: %v", err)
	}
}

func TestCommandRunnerNoCommands(t *testing.T) {
	if err := NewCommandRunner(nil).Run(); err != nil {
		t.Fatalf("Run with no commands: %v", err)
	}
}

// TestRunInteractiveStdin verifies an interactive command can answer a prompt
// read from its stdin (e.g. apt's y/n confirmation).
func TestRunInteractiveStdin(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "ask", CmdLine: "read x && echo got:$x", Interactive: true},
	})
	var out, errOut bytes.Buffer
	if err := r.runInteractive(r.Commands[0], strings.NewReader("hello\n"), &out, &errOut); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}
	if !strings.Contains(out.String(), "got:hello") {
		t.Fatalf("expected 'got:hello' in output, got: %q", out.String())
	}
}

func TestRunInteractiveFailure(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "boom", CmdLine: "echo oops 1>&2; exit 3", Interactive: true},
	})
	var out, errOut bytes.Buffer
	err := r.runInteractive(r.Commands[0], strings.NewReader(""), &out, &errOut)
	if err == nil {
		t.Fatal("expected error from interactive command")
	}
	if !strings.Contains(err.Error(), "exit code 3") {
		t.Fatalf("error should mention exit code 3, got: %v", err)
	}
	if !strings.Contains(errOut.String(), "oops") {
		t.Fatalf("stderr should be written through, got: %q", errOut.String())
	}
}

// TestRunInteractiveMixed verifies a mix of normal and interactive commands in
// the non-TTY path: interactive commands stream directly, normal ones go
// through the buffer.
func TestRunInteractiveMixed(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "normal", CmdLine: "echo one"},
		{Name: "direct", CmdLine: "echo two", Interactive: true},
		{Name: "normal2", CmdLine: "echo three"},
	})
	if err := r.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestRunInteractiveInterrupt verifies a failing interactive command aborts the
// rest of the sequence and reports the exit code.
func TestRunInteractiveInterrupt(t *testing.T) {
	r := NewCommandRunner([]Command{
		{Name: "ok", CmdLine: "echo fine"},
		{Name: "boom", CmdLine: "exit 4", Interactive: true},
		{Name: "never", CmdLine: "echo should not run", Interactive: true},
	})
	err := r.Run()
	if err == nil {
		t.Fatal("expected error from failing interactive command")
	}
	if !strings.Contains(err.Error(), "exit code 4") {
		t.Fatalf("error should mention exit code 4, got: %v", err)
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct{ in, want string }{
		{"\x1b[31mRED\x1b[0m text", "RED text"},
		{"plain", "plain"},
		{"\x1b[2K line\r", " line\r"},
		{"a\x1b[?25lb\x1b[?25h", "ab"},
	}
	for _, tt := range tests {
		if got := stripANSI(tt.in); got != tt.want {
			t.Fatalf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRunCommandPTY verifies the pty-backed execution path: output streams
// line-buffered, ANSI is stripped, and exit codes propagate.
func TestRunCommandPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty requires POSIX")
	}
	r := NewCommandRunner([]Command{
		{Name: "c", CmdLine: "printf '\\e[31mRED\\e[0m hello\\nworld\\n'"},
	})
	r.usePTY = true

	var lines []string
	printerDone := make(chan struct{})
	go func() {
		defer close(printerDone)
		for l := range r.output {
			lines = append(lines, l)
		}
	}()
	statusDone := make(chan struct{})
	go func() {
		defer close(statusDone)
		for range r.done {
		}
	}()

	if err := r.runCommandPTY(r.Commands[0]); err != nil {
		t.Fatalf("runCommandPTY: %v", err)
	}
	close(r.output)
	close(r.done)
	<-printerDone
	<-statusDone

	want := []string{"RED hello", "world"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines (%q), want %q", len(lines), lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestRunCommandPTYFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty requires POSIX")
	}
	r := NewCommandRunner([]Command{{Name: "boom", CmdLine: "echo oops 1>&2; exit 3"}})
	r.usePTY = true

	go func() {
		for range r.output {
		}
	}()
	statusDone := make(chan struct{})
	go func() {
		defer close(statusDone)
		for range r.done {
		}
	}()

	err := r.runCommandPTY(r.Commands[0])
	close(r.output)
	close(r.done)
	<-statusDone
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exit code 3") {
		t.Fatalf("error should mention exit code 3, got: %v", err)
	}
}
