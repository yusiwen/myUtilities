package runner

import (
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
