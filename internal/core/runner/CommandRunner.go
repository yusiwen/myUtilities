package runner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/morikuni/aec"
)

type Command struct {
	Name        string `help:"Description of this command" default:""`
	CmdLine     string `help:"Command line" default:""`
	Interactive bool   // run with stdin/stdout/stderr connected directly to the terminal
}

type CmdStatus struct {
	isSuccess bool
	exitCode  int
	errMsg    string
}

var outputColor aec.ANSI
var errColor aec.ANSI

const (
	ANSI_CLEAR_LINE    = "\033[2K"
	ANSI_MOVE_UP       = "\033[1A"
	ANSI_MOVE_UP_LINES = "\033[%dA"
)

// ansiRe strips CSI/OSC escape sequences from child output drawn into the
// gray display area, so a pty-backed child that colorizes still renders
// uniformly.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\].*?(\x07|\x1b\\)|\x1b[@-_]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func init() {
	// As recommended on https://no-color.org/
	if v := os.Getenv("NO_COLOR"); v != "" {
		// nil values will result in no ANSI color codes being emitted.
		return
	} else if runtime.GOOS == "windows" {
		outputColor = aec.CyanF
		errColor = aec.RedF
	} else {
		outputColor = aec.BlueF
		errColor = aec.RedF
	}
}

// isTerminal reports whether f refers to a character device (a TTY), as
// opposed to a pipe or file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func (r *CommandRunner) Run() error {
	if len(r.Commands) == 0 {
		return nil
	}

	r.usePTY = isTerminal(os.Stdout) && runtime.GOOS != "windows"

	if !isTerminal(os.Stdout) {
		return r.runPlain()
	}

	r.wg.Add(3)
	go r.runCommands()
	go r.d.refreshBuffer()
	go r.d.update()

	r.wg.Wait()
	return r.err
}

// runPlain executes the commands sequentially without the ANSI display
// machinery, suitable for piped or redirected output. The unbuffered
// output/done channels are drained by helper goroutines since the display
// consumer is not running.
func (r *CommandRunner) runPlain() error {
	printerDone := make(chan struct{})
	statusDone := make(chan struct{})
	go func() {
		defer close(printerDone)
		for line := range r.output {
			fmt.Println(line)
		}
	}()
	go func() {
		defer close(statusDone)
		for range r.done {
		}
	}()

	finish := func() {
		close(r.output)
		close(r.done)
		<-printerDone
		<-statusDone
	}

	for _, cmd := range r.Commands {
		name := cmd.Name
		if name == "" {
			name = cmd.CmdLine
		}
		fmt.Printf("Executing [%s]...\n", name)
		var err error
		if cmd.Interactive {
			err = r.runInteractive(cmd, os.Stdin, os.Stdout, os.Stderr)
		} else {
			err = r.runCommand(cmd)
		}
		if err != nil {
			finish()
			return err
		}
		fmt.Printf("%s done\n", name)
	}
	finish()
	return nil
}

func (r *CommandRunner) runCommands() {
	defer r.wg.Done()
	defer close(r.output)
	defer close(r.done)

	for _, cmd := range r.Commands {
		out := fmt.Sprintf("Executing [%s]...", cmd.Name)
		fmt.Println(aec.Apply(out, outputColor))

		if cmd.Interactive {
			// Suspend the redraw display so it does not interfere with the
			// interactive session, then stream the command directly.
			r.d.pause()
			err := r.runInteractive(cmd, os.Stdin, os.Stdout, os.Stderr)
			r.d.resume()

			if err != nil {
				r.err = err
				fmt.Println(aec.Apply("Error:", errColor))
				break
			}
			fmt.Printf(ANSI_MOVE_UP)
			out = fmt.Sprintf("%s done", out)
			fmt.Print(ANSI_CLEAR_LINE)
			fmt.Println(aec.Apply(out, outputColor))
			continue
		}

		err := r.runCommand(cmd)
		<-r.d.clear

		if err != nil {
			r.err = err
			// Keep the failed command's recent output visible before the
			// error is printed, since the display cleanup wiped it.
			for _, l := range r.d.failedOut {
				fmt.Println(l)
			}
			fmt.Println(aec.Apply("Error:", errColor))
			break
		} else {
			fmt.Printf(ANSI_MOVE_UP)
			out = fmt.Sprintf("%s done", out)
			fmt.Print(ANSI_CLEAR_LINE)
			fmt.Println(aec.Apply(out, outputColor))
		}
	}
}

// runCommand dispatches a non-interactive command: on a terminal the child
// runs on a pty so stdio programs line-buffer and stream promptly; elsewhere
// (piped output or platforms without pty) it uses plain pipes.
func (r *CommandRunner) runCommand(command Command) error {
	if r.usePTY {
		return r.runCommandPTY(command)
	}
	return r.runCommandPipes(command)
}

func (r *CommandRunner) runCommandPipes(command Command) error {
	cmd := exec.Command("bash", "-c", command.CmdLine)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil {
		return err
	}

	stderrCh := make(chan string, 1)
	go func() {
		errMsgBytes, err := io.ReadAll(stderr)
		if err != nil {
			log.Printf("Failed to read stderr: %v", err)
		}
		stderrCh <- string(errMsgBytes)
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		r.output <- scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Output reading error: %v", err)
	}

	errorMsg := <-stderrCh

	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			msg := errorMsg
			if strings.TrimSpace(msg) == "" {
				msg = "no error output"
			}
			r.done <- &CmdStatus{
				isSuccess: false,
				exitCode:  exitError.ExitCode(),
				errMsg:    msg,
			}
			return fmt.Errorf("command %q failed (exit code %d): %s", command.CmdLine, exitError.ExitCode(), msg)
		}
		r.done <- &CmdStatus{
			isSuccess: false,
			exitCode:  -1,
			errMsg:    err.Error(),
		}
		return fmt.Errorf("command %q failed to run: %w", command.CmdLine, err)
	}
	r.done <- &CmdStatus{
		isSuccess: true,
		exitCode:  0,
	}

	return nil
}

// runCommandPTY runs a command with its stdout/stderr on a pty, feeding each
// (ANSI-stripped) line into the display buffer. stdin stays /dev/null so
// commands that read it still fail fast.
func (r *CommandRunner) runCommandPTY(command Command) error {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	cmd := exec.Command("bash", "-c", command.CmdLine)
	cmd.Stdout = tty
	cmd.Stderr = tty
	if err := cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return err
	}
	// The child holds the slave; close our copy so the master reaches EOF
	// once the process exits.
	tty.Close()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	scanner := bufio.NewScanner(ptmx)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(stripANSI(scanner.Text()), "\r")
		r.output <- line
	}
	// On Linux, reading the master after the slave closes yields EIO, which
	// signals end of output rather than a real error.
	if err := scanner.Err(); err != nil && !errors.Is(err, syscall.EIO) {
		log.Printf("Output reading error: %v", err)
	}
	ptmx.Close()

	if err := <-waitCh; err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			r.done <- &CmdStatus{
				isSuccess: false,
				exitCode:  exitError.ExitCode(),
				errMsg:    "no error output",
			}
			return fmt.Errorf("command %q failed (exit code %d)", command.CmdLine, exitError.ExitCode())
		}
		r.done <- &CmdStatus{
			isSuccess: false,
			exitCode:  -1,
			errMsg:    err.Error(),
		}
		return fmt.Errorf("command %q failed to run: %w", command.CmdLine, err)
	}
	r.done <- &CmdStatus{
		isSuccess: true,
		exitCode:  0,
	}
	return nil
}

// runInteractive runs a command with stdin/stdout/stderr wired directly to the
// given streams, bypassing the display buffer. It is used for commands that
// prompt the user (sudo, ssh, apt confirmations).
func (r *CommandRunner) runInteractive(command Command, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command("bash", "-c", command.CmdLine)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("command %q failed (exit code %d)", command.CmdLine, exitError.ExitCode())
		}
		return fmt.Errorf("command %q failed to run: %w", command.CmdLine, err)
	}
	return nil
}

type CommandRunner struct {
	output chan string
	done   chan *CmdStatus

	Commands []Command

	usePTY bool

	err error
	wg  *sync.WaitGroup
	d   *display
}

func NewCommandRunner(commands []Command) *CommandRunner {
	output := make(chan string)
	done := make(chan *CmdStatus)
	clearDone := make(chan struct{})
	redraw := make(chan struct{}, 1)
	wg := sync.WaitGroup{}

	return &CommandRunner{
		Commands: commands,
		output:   output,
		done:     done,
		wg:       &wg,
		d: &display{
			output:    output,
			done:      done,
			clear:     clearDone,
			redraw:    redraw,
			wg:        &wg,
			cmdCnt:    len(commands),
			doneCnt:   0,
			isHidden:  true,
			prevLines: 0,
			buffer:    make([]string, 0),
		},
	}
}

type display struct {
	output chan string
	done   chan *CmdStatus
	clear  chan struct{}
	redraw chan struct{}

	wg *sync.WaitGroup

	cmdCnt  int
	doneCnt int

	isHidden    bool
	prevLines   int
	buffer      []string
	bufferMutex sync.Mutex

	// isPaused suspends redraws while an interactive command is running.
	isPaused bool

	// failedOut holds a snapshot of buffer for the last command that failed,
	// captured before cleanUp wipes it, so runCommands can re-print the
	// failed command's output next to the error.
	failedOut []string
}

func (d *display) update() {
	defer d.wg.Done()
	defer close(d.clear)

	for {
		select {
		case <-d.redraw:
			d.print()
		case status := <-d.done:
			// runCommands closes r.done once all commands have finished;
			// a nil status signals shutdown (e.g. the last command was
			// interactive and never sent a status).
			if status == nil {
				return
			}
			if !status.isSuccess {
				// Snapshot the buffer before cleanup erases it, so the
				// failed command's output can be re-printed with the error.
				d.bufferMutex.Lock()
				d.failedOut = append([]string(nil), d.buffer...)
				d.bufferMutex.Unlock()
			}
			d.cleanUp()

			d.doneCnt++
			if !status.isSuccess || d.doneCnt == d.cmdCnt {
				return
			}
		}
	}
}

func (d *display) refreshBuffer() {
	defer d.wg.Done()
	for line := range d.output {
		d.bufferMutex.Lock()
		d.buffer = append(d.buffer, line)
		if len(d.buffer) > 6 {
			d.buffer = d.buffer[1:]
		}
		d.bufferMutex.Unlock()
		// Ask for an immediate redraw; coalesce bursts with a non-blocking
		// send into a size-1 channel.
		select {
		case d.redraw <- struct{}{}:
		default:
		}
	}
}

func (d *display) print() {
	d.bufferMutex.Lock()
	defer d.bufferMutex.Unlock()
	if d.isPaused {
		return
	}
	if d.prevLines > 0 {
		fmt.Printf(ANSI_MOVE_UP_LINES, d.prevLines)
	}

	currentLines := len(d.buffer)
	if currentLines > 0 {
		d.isHidden = false
	}
	for _, l := range d.buffer {
		out := aec.Apply(l, aec.Faint)
		fmt.Print(ANSI_CLEAR_LINE + " " + out + "\n")
	}

	d.prevLines = currentLines
}

// clearScreenLocked erases the transient redraw and resets the buffer state.
// The caller must hold bufferMutex.
func (d *display) clearScreenLocked() {
	if !d.isHidden {
		fmt.Printf(ANSI_MOVE_UP_LINES, d.prevLines)
		for i := 0; i < d.prevLines; i++ {
			fmt.Println(ANSI_CLEAR_LINE)
		}
		fmt.Printf(ANSI_MOVE_UP_LINES, d.prevLines)
	}
	// Reset the buffer unconditionally: a command that finished before the
	// first ticker tick never triggered print(), leaving isHidden true and
	// the buffer stale for the next command.
	d.prevLines = 0
	d.buffer = nil
	d.isHidden = true
}

func (d *display) cleanUp() {
	d.bufferMutex.Lock()
	d.clearScreenLocked()
	d.bufferMutex.Unlock()
	d.clear <- struct{}{}
}

// pause clears the screen and stops redraws so an interactive command can take
// over the terminal without the display moving the cursor around.
func (d *display) pause() {
	d.bufferMutex.Lock()
	if !d.isPaused {
		d.clearScreenLocked()
		d.isPaused = true
	}
	d.bufferMutex.Unlock()
}

// resume re-enables redraws after an interactive command finishes.
func (d *display) resume() {
	d.bufferMutex.Lock()
	d.isPaused = false
	d.bufferMutex.Unlock()
}
