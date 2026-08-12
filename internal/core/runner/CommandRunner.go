package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/morikuni/aec"
	"github.com/tonistiigi/vt100"
	"golang.org/x/term"
)

type Command struct {
	Name        string        `help:"Description of this command" default:""`
	CmdLine     string        `help:"Command line" default:""`
	Interactive bool          // run with stdin/stdout/stderr connected directly to the terminal
	Env         []string      // extra environment variables in KEY=VALUE form
	Dir         string        // working directory for the command
	Timeout     time.Duration // optional per-command timeout; zero means no timeout
}

type CmdStatus struct {
	isSuccess bool
	exitCode  int
	errMsg    string
}

var outputColor aec.ANSI
var errColor aec.ANSI
var successColor aec.ANSI

const (
	ANSI_CLEAR_LINE    = "\033[2K"
	ANSI_CLEAR_DOWN    = "\033[J"
	ANSI_MOVE_UP       = "\033[1A"
	ANSI_MOVE_UP_LINES = "\033[%dA"
)

// spinnerFrames is the animation cycle shown on the step header while a
// non-interactive command is running.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ErrInterrupted is returned by Run when the user interrupted execution
// (Ctrl-C). Callers may use it to pick an exit code (e.g. 128+SIGINT).
var ErrInterrupted = errors.New("interrupted")

// formatElapsed renders a duration for the step header/final line:
// "<1s" as e.g. "0.4s", "<1m" as "1.5s", otherwise "1m05s".
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// regionPad reserves columns between the terminal edge and the vt100 width so
// a rendered row (plus its leading space) never wraps on the real terminal.
const regionPad = 3

// logLinesDefault is the per-step region height in rows, matching buildkit's
// termHeightMin. MU_RUN_LOG_LINES overrides it, mirroring BUILDKIT_TTY_LOG_LINES.
const logLinesDefault = 6

// logLines returns the region height: MU_RUN_LOG_LINES if set and valid, else
// logLinesDefault.
func logLines() int {
	if v := os.Getenv("MU_RUN_LOG_LINES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return logLinesDefault
}

// termWidth returns the stdout terminal width, falling back to 80.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func init() {
	// As recommended on https://no-color.org/
	if v := os.Getenv("NO_COLOR"); v != "" {
		// nil values will result in no ANSI color codes being emitted.
		return
	} else if runtime.GOOS == "windows" {
		outputColor = aec.CyanF
		errColor = aec.RedF
		successColor = aec.GreenF
	} else {
		outputColor = aec.BlueF
		errColor = aec.RedF
		successColor = aec.GreenF
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Watch for the first interrupt: mark the run as interrupted and forward
	// the signal to the currently executing child (a pty child is in its own
	// session, so the terminal-generated SIGINT never reaches it). A second
	// Ctrl-C hits the default disposition (NotifyContext unsubscribes after
	// the first signal) and kills the process outright.
	go func() {
		<-ctx.Done()
		r.interrupted.Store(true)
		r.signalActive(os.Interrupt)
	}()

	r.usePTY = isTerminal(os.Stdout) && runtime.GOOS != "windows"

	if !isTerminal(os.Stdout) {
		err := r.runPlain()
		if r.interrupted.Load() {
			return ErrInterrupted
		}
		return err
	}

	r.d.width = termWidth()

	r.wg.Add(3)
	go r.runCommands()
	go r.d.feedOutput()
	go r.d.update()

	r.wg.Wait()
	if r.interrupted.Load() {
		return ErrInterrupted
	}
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
		if r.interrupted.Load() {
			break
		}
		name := cmd.Name
		if name == "" {
			name = cmd.CmdLine
		}
		fmt.Printf("Executing [%s]...\n", name)
		start := time.Now()
		var err error
		if cmd.Interactive {
			err = r.runInteractive(cmd, os.Stdin, os.Stdout, os.Stderr)
		} else {
			err = r.runCommand(cmd)
		}
		elapsed := time.Since(start)
		if err != nil {
			if r.interrupted.Load() {
				fmt.Println("Interrupted.")
				break
			}
			finish()
			return err
		}
		fmt.Printf("%s ✓ (%s)\n", name, formatElapsed(elapsed))
	}
	finish()
	return nil
}

func (r *CommandRunner) runCommands() {
	defer r.wg.Done()
	defer close(r.output)
	defer close(r.done)

	for _, cmd := range r.Commands {
		if r.interrupted.Load() {
			break
		}

		if cmd.Interactive {
			// The header is printed here because the display is suspended
			// during the interactive session and cannot render it.
			out := fmt.Sprintf("Executing [%s]...", cmd.Name)
			fmt.Println(aec.Apply(out, outputColor))

			r.interactiveStart = time.Now()
			// Suspend the redraw display so it does not interfere with the
			// interactive session, then stream the command directly.
			r.d.pause()
			err := r.runInteractive(cmd, os.Stdin, os.Stdout, os.Stderr)
			r.d.resume()

			if err != nil {
				if r.interrupted.Load() {
					fmt.Println("Interrupted.")
					break
				}
				r.err = err
				fmt.Println(aec.Apply("Error:", errColor))
				break
			}
			elapsed := time.Since(r.interactiveStart)
			fmt.Printf(ANSI_MOVE_UP)
			fmt.Print(ANSI_CLEAR_LINE)
			fmt.Println(aec.Apply(fmt.Sprintf("Executing [%s]... ✓ %s", cmd.Name, formatElapsed(elapsed)), successColor))
			continue
		}

		r.d.startStep(cmd.Name)
		err := r.runCommand(cmd)
		<-r.d.clear

		if err != nil {
			if r.interrupted.Load() {
				fmt.Println("Interrupted.")
				break
			}
			r.err = err
			elapsed := time.Since(r.d.stepStart)
			fmt.Println(aec.Apply(fmt.Sprintf("Executing [%s]... ✗ %s", cmd.Name, formatElapsed(elapsed)), errColor))
			// Keep the failed command's recent output visible before the
			// error is printed, since the display cleanup wiped it.
			for _, l := range r.d.failedOut {
				fmt.Println(l)
			}
			fmt.Println(aec.Apply("Error:", errColor))
			break
		} else {
			elapsed := time.Since(r.d.stepStart)
			fmt.Println(aec.Apply(fmt.Sprintf("Executing [%s]... ✓ %s", cmd.Name, formatElapsed(elapsed)), successColor))
		}
	}
}

// ParseCommandSpec decodes a single command spec of the form
// "[<name>::][!]<command line>": an optional name separated by "::", and an
// optional leading "!" marking the command as interactive. It is shared by the
// CLI mapper and recipe loading.
func ParseCommandSpec(val string) Command {
	var c Command
	if name, cmd, found := strings.Cut(val, "::"); found {
		c.Name = name
		c.CmdLine = cmd
	} else {
		c.CmdLine = val
	}
	if strings.HasPrefix(c.CmdLine, "!") {
		c.Interactive = true
		c.CmdLine = strings.TrimPrefix(c.CmdLine, "!")
	}
	return c
}

// newBashCommand builds the exec.Cmd for a command line, applying the
// command's environment, working directory, and timeout. The returned context
// is cancelled when the command finishes; if it is a WithTimeout context, the
// process is killed once the deadline passes.
func newBashCommand(command Command) (*exec.Cmd, context.Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc
	if command.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), command.Timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", command.CmdLine)
	cmd.Env = append(os.Environ(), command.Env...)
	cmd.Dir = command.Dir
	return cmd, ctx, cancel
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
	cmd, ctx, cancel := newBashCommand(command)
	defer cancel()
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
	r.setActive(cmd)
	defer r.setActive(nil)

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
		if ctx.Err() != nil {
			return ctx.Err()
		}
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

// runCommandPTY runs a command with its stdout/stderr on a pty, feeding raw
// output chunks into the display's VT100 emulator (which handles wrapping,
// ANSI and CR like a real terminal). stdin stays /dev/null so commands that
// read it still fail fast.
func (r *CommandRunner) runCommandPTY(command Command) error {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return err
	}
	cmd, ctx, cancel := newBashCommand(command)
	defer cancel()
	cmd.Stdout = tty
	cmd.Stderr = tty
	if err := cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return err
	}
	r.setActive(cmd)
	defer r.setActive(nil)
	// The child holds the slave; close our copy so the master reaches EOF
	// once the process exits.
	tty.Close()

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	buf := make([]byte, 32*1024)
	reader := bufio.NewReader(ptmx)
	for {
		n, rerr := reader.Read(buf)
		if n > 0 {
			r.output <- string(buf[:n])
		}
		if rerr != nil {
			// On Linux, reading the master after the slave closes yields EIO,
			// which signals end of output rather than a real error.
			if errors.Is(rerr, syscall.EIO) || errors.Is(rerr, io.EOF) {
				break
			}
			log.Printf("Output reading error: %v", rerr)
			break
		}
	}
	ptmx.Close()

	if err := <-waitCh; err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
	cmd, ctx, cancel := newBashCommand(command)
	defer cancel()
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("command %q failed to run: %w", command.CmdLine, err)
	}
	r.setActive(cmd)
	defer r.setActive(nil)
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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

	interrupted atomic.Bool

	activeMu sync.Mutex
	active   *exec.Cmd

	interactiveStart time.Time
}

// setActive records the currently executing child so the interrupt handler can
// forward the signal to it.
func (r *CommandRunner) setActive(cmd *exec.Cmd) {
	r.activeMu.Lock()
	r.active = cmd
	r.activeMu.Unlock()
}

// signalActive sends sig to the currently executing child, if any.
func (r *CommandRunner) signalActive(sig os.Signal) {
	r.activeMu.Lock()
	cmd := r.active
	r.activeMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(sig)
	}
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
			output:   output,
			done:     done,
			clear:    clearDone,
			redraw:   redraw,
			wg:       &wg,
			isHidden: true,
			prevRows: 0,
			width:    80,
			maxRows:  logLines(),
		},
	}
}

type display struct {
	output chan string
	done   chan *CmdStatus
	clear  chan struct{}
	redraw chan struct{}

	wg *sync.WaitGroup

	width   int
	maxRows int

	isHidden bool
	prevRows int
	term     *vt100.VT100

	bufferMutex sync.Mutex

	// stepActive is true while a non-interactive command is running; the
	// spinner ticker only draws while it is set.
	stepActive bool
	stepName   string
	stepStart  time.Time
	spinnerIdx int

	// isPaused suspends redraws while an interactive command is running.
	isPaused bool

	// failedOut holds a snapshot of the emulated screen for the last command
	// that failed, captured before cleanUp wipes it, so runCommands can
	// re-print the failed command's output next to the error.
	failedOut []string
}

func (d *display) update() {
	defer d.wg.Done()
	defer close(d.clear)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-d.redraw:
			d.print()
		case <-ticker.C:
			d.spin()
		case status := <-d.done:
			// runCommands / recipe runners close r.done once everything has
			// finished; a nil status signals shutdown. Unlike before, update
			// does NOT exit on a failed command: retries and keep-going runs
			// continue to use the display across failures.
			if status == nil {
				return
			}
			if !status.isSuccess {
				// Snapshot the emulated screen before cleanup erases it, so
				// the failed command's output can be re-printed with the error.
				d.bufferMutex.Lock()
				d.failedOut = d.snapshotRowsLocked()
				d.bufferMutex.Unlock()
			}
			d.cleanUp()
		}
	}
}

// spin advances the spinner frame and redraws the step header. It is called on
// a fixed ticker while a non-interactive command is running.
func (d *display) spin() {
	d.bufferMutex.Lock()
	if !d.stepActive || d.isPaused {
		d.bufferMutex.Unlock()
		return
	}
	d.spinnerIdx++
	d.printLocked()
	d.bufferMutex.Unlock()
}

// startStep begins a new non-interactive command step: it prints the initial
// header line (with spinner and elapsed time) and resets the emulated terminal.
func (d *display) startStep(name string) {
	d.bufferMutex.Lock()
	defer d.bufferMutex.Unlock()
	d.stepName = name
	d.stepStart = time.Now()
	d.stepActive = true
	d.spinnerIdx = 0
	d.term = vt100.NewVT100(d.maxRows, d.width-regionPad)
	d.prevRows = 0
	d.isHidden = false
	d.failedOut = nil
	d.printLocked()
}

// feedOutput consumes raw output chunks from the runner and feeds them into
// the VT100 emulator, asking for an immediate redraw per chunk.
func (d *display) feedOutput() {
	defer d.wg.Done()
	for chunk := range d.output {
		d.bufferMutex.Lock()
		if d.term != nil {
			d.term.Write([]byte(chunk)) // ignore error: never trust vt100
		}
		d.bufferMutex.Unlock()
		// Coalesce bursts with a non-blocking send into a size-1 channel.
		select {
		case d.redraw <- struct{}{}:
		default:
		}
	}
}

// snapshotRowsLocked returns the non-empty rows of the emulated screen. The
// caller must hold bufferMutex.
func (d *display) snapshotRowsLocked() []string {
	if d.term == nil {
		return nil
	}
	var rows []string
	for _, row := range d.term.Content {
		if !isEmpty(row) {
			rows = append(rows, string(row))
		}
	}
	return rows
}

// isEmpty reports whether a vt100 screen row is entirely blank.
func isEmpty(row []rune) bool {
	for _, r := range row {
		if r != ' ' {
			return false
		}
	}
	return true
}

func (d *display) print() {
	d.bufferMutex.Lock()
	defer d.bufferMutex.Unlock()
	d.printLocked()
}

// printLocked renders the step header plus the emulated region rows. The
// caller must hold bufferMutex.
func (d *display) printLocked() {
	if d.isPaused || d.term == nil || !d.stepActive {
		return
	}

	rows := d.snapshotRowsLocked()
	if d.prevRows > 0 {
		fmt.Printf(ANSI_MOVE_UP_LINES, d.prevRows)
	}
	if len(rows) > 0 {
		d.isHidden = false
	}
	// Header line: "Executing [<name>]... <spinner> <elapsed>"
	elapsed := time.Since(d.stepStart)
	header := fmt.Sprintf("Executing [%s]... %s %s", d.stepName, spinnerFrames[d.spinnerIdx%len(spinnerFrames)], formatElapsed(elapsed))
	fmt.Print(ANSI_CLEAR_LINE + aec.Apply(header, outputColor) + "\n")
	total := 1 + len(rows)
	for _, row := range rows {
		fmt.Print(ANSI_CLEAR_LINE + " " + aec.Apply(row, aec.Faint) + "\n")
	}
	// Erase rows left over from a taller previous draw.
	if total < d.prevRows {
		fmt.Print(ANSI_CLEAR_DOWN)
	}
	d.prevRows = total
}

// clearScreenLocked erases the transient redraw and resets the display state.
// The caller must hold bufferMutex.
func (d *display) clearScreenLocked() {
	if !d.isHidden && d.prevRows > 0 {
		fmt.Printf(ANSI_MOVE_UP_LINES, d.prevRows)
		// Return to column 0, clear this line and everything below, wiping
		// the whole region regardless of exact row count.
		fmt.Print("\r" + ANSI_CLEAR_LINE + ANSI_CLEAR_DOWN)
	}
	d.prevRows = 0
	d.isHidden = true
	d.stepActive = false
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
