package runner

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0.0s"},
		{400 * time.Millisecond, "0.4s"},
		{1500 * time.Millisecond, "1.5s"},
		{10 * time.Second, "10.0s"},
		{65 * time.Second, "1m05s"},
		{125 * time.Second, "2m05s"},
	}
	for _, c := range cases {
		if got := formatElapsed(c.d); got != c.want {
			t.Fatalf("formatElapsed(%v) = %q, want %q", c.d, got, c.want)
		}
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

func TestLogLinesEnv(t *testing.T) {
	old := os.Getenv("MU_RUN_LOG_LINES")
	defer os.Setenv("MU_RUN_LOG_LINES", old)

	for _, tt := range []struct {
		env  string
		want int
	}{
		{"", 6},
		{"10", 10},
		{"abc", 6},
		{"0", 6},
		{"-3", 6},
	} {
		os.Setenv("MU_RUN_LOG_LINES", tt.env)
		if got := logLines(); got != tt.want {
			t.Fatalf("logLines(%q) = %d, want %d", tt.env, got, tt.want)
		}
	}
}

// TestDisplaySoftWrap verifies the VT100-backed display soft-wraps long lines
// at the emulated width instead of truncating, and that ANSI/CR are handled
// by the emulator.
func TestDisplaySoftWrap(t *testing.T) {
	newDisplay := func() *display {
		d := &display{width: 20, maxRows: 6}
		d.startStep("test")
		return d
	}

	// A 40-char line wraps into multiple rows of the emulated width.
	d := newDisplay()
	d.term.Write([]byte(strings.Repeat("x", 40) + "\n"))
	rows := d.snapshotRowsLocked()
	if len(rows) < 2 {
		t.Fatalf("got %d rows, want a 40-col line to wrap", len(rows))
	}
	for i, row := range rows {
		if len([]rune(row)) != d.term.Width {
			t.Fatalf("row %d width = %d, want emulated width %d", i, len([]rune(row)), d.term.Width)
		}
	}

	// ANSI codes must be interpreted, not leaked into the screen.
	d = newDisplay()
	d.term.Write([]byte("\x1b[31mRED\x1b[0m text\n"))
	rows = d.snapshotRowsLocked()
	joined := strings.Join(rows, "")
	if strings.Contains(joined, "\x1b") {
		t.Fatalf("ANSI leaked into emulated screen: %q", rows)
	}
	if !strings.Contains(joined, "RED text") {
		t.Fatalf("content missing after ANSI: %q", rows)
	}

	// Carriage returns overwrite in place (progress-bar style).
	d = newDisplay()
	d.term.Write([]byte("spin 0%\rspin 50%\rspin 100%\n"))
	rows = d.snapshotRowsLocked()
	joined = strings.Join(rows, "")
	if strings.Contains(joined, "spin 0%") || !strings.Contains(joined, "spin 100%") {
		t.Fatalf("CR overwrite not reflected: %q", rows)
	}

	// More than maxRows lines scroll the emulated screen.
	d = newDisplay()
	for i := 0; i < 10; i++ {
		d.term.Write([]byte("line\n"))
	}
	if got := d.term.UsedHeight(); got > d.maxRows {
		t.Fatalf("UsedHeight %d exceeds maxRows %d", got, d.maxRows)
	}
}

// TestRunCommandPTY verifies the pty-backed execution path: output streams
// line-buffered, ANSI is interpreted by the emulator, and exit codes propagate.
func TestRunCommandPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty requires POSIX")
	}
	r := NewCommandRunner([]Command{
		{Name: "c", CmdLine: "printf '\\e[31mRED\\e[0m hello\\nworld\\n'"},
	})
	r.usePTY = true

	var chunks []string
	printerDone := make(chan struct{})
	go func() {
		defer close(printerDone)
		for c := range r.output {
			chunks = append(chunks, c)
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

	joined := strings.Join(chunks, "")
	if !strings.Contains(joined, "RED") || !strings.Contains(joined, "hello") || !strings.Contains(joined, "world") {
		t.Fatalf("output chunks missing content: %q", joined)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one output chunk")
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
