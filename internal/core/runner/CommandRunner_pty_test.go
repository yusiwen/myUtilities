package runner

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestPtyInteractive is an integration test that gives an interactive command
// a real controlling terminal (via the system "script" utility and a pty) and
// drives input into it, covering the /dev/tty path used by sudo/ssh/passwd.
// It runs only on POSIX where "script" exists.
func TestPtyInteractive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty integration test requires a POSIX platform")
	}
	scriptBin, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script not available")
	}

	// Re-exec the test binary as a helper that runs the interactive command;
	// "script" gives it a pty and forwards our stdin to it as the terminal.
	helper := exec.Command(os.Args[0], "-test.run=TestPtyHelperProcess")

	script := exec.Command(scriptBin, "-qec", helper.String(), "/dev/null")
	script.Env = append(os.Environ(), "MU_RUN_PTY_HELPER=1")
	script.Stdin = strings.NewReader("hello\n")
	var out bytes.Buffer
	script.Stdout = &out
	script.Stderr = &out
	if err := script.Run(); err != nil {
		t.Fatalf("script failed: %v; output:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "got:hello") {
		t.Fatalf("expected 'got:hello' echoed from /dev/tty input, output:\n%s", out.String())
	}
}

// TestPtyHelperProcess is not a real test: when MU_RUN_PTY_HELPER is set it
// runs an interactive command through the full CommandRunner.Run() (including
// the TTY display machinery) against the pty provided by TestPtyInteractive
// and exits; otherwise it is a no-op.
func TestPtyHelperProcess(t *testing.T) {
	if os.Getenv("MU_RUN_PTY_HELPER") != "1" {
		return
	}
	r := NewCommandRunner([]Command{
		{Name: "ask", CmdLine: "read x < /dev/tty && echo got:$x", Interactive: true},
	})
	if err := r.Run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
