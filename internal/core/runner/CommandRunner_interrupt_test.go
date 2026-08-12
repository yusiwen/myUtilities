package runner

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestRunInterrupt verifies that sending SIGINT while a long-running command is
// executing forwards the signal to the child and Run returns ErrInterrupted
// instead of a hard failure. It exercises the non-TTY (plain) path, since test
// stdout is never a terminal.
func TestRunInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal semantics differ on windows")
	}
	r := NewCommandRunner([]Command{
		{Name: "long", CmdLine: "sleep 30"},
		{Name: "after", CmdLine: "echo should not run"},
	})

	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			r.activeMu.Lock()
			active := r.active != nil
			r.activeMu.Unlock()
			if active {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	err := r.Run()
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Run() error = %v, want ErrInterrupted", err)
	}
	if !r.interrupted.Load() {
		t.Fatal("interrupted flag not set")
	}
}
