package runner

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/morikuni/aec"
)

type Command struct {
	Name    string `help:"Description of this command" default:""`
	CmdLine string `help:"Command line" default:""`
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
		if err := r.runCommand(cmd); err != nil {
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
			fmt.Printf("%v\n", err)
			break
		} else {
			fmt.Printf(ANSI_MOVE_UP)
			out = fmt.Sprintf("%s done", out)
			fmt.Print(ANSI_CLEAR_LINE)
			fmt.Println(aec.Apply(out, outputColor))
		}
	}
}

func (r *CommandRunner) runCommand(command Command) error {
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

type CommandRunner struct {
	output chan string
	done   chan *CmdStatus

	Commands []Command

	err error
	wg  *sync.WaitGroup
	d   *display
}

func NewCommandRunner(commands []Command) *CommandRunner {
	output := make(chan string)
	done := make(chan *CmdStatus)
	clearDone := make(chan struct{})
	wg := sync.WaitGroup{}

	return &CommandRunner{
		Commands: commands,
		output:   output,
		done:     done,
		wg:       &wg,
		d: &display{
			output:    output,
			done:      done,
			wg:        &wg,
			clear:     clearDone,
			cmdCnt:    len(commands),
			doneCnt:   0,
			isHidden:  true,
			prevLines: 0,
			buffer:    make([]string, 0),
			ticker:    time.NewTicker(200 * time.Millisecond),
		},
	}
}

type display struct {
	output chan string
	done   chan *CmdStatus
	clear  chan struct{}
	ticker *time.Ticker

	wg *sync.WaitGroup

	cmdCnt  int
	doneCnt int

	isHidden    bool
	prevLines   int
	buffer      []string
	bufferMutex sync.Mutex

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
		case <-d.ticker.C:
			d.print()
		case status := <-d.done:
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
	}
}

func (d *display) print() {
	d.bufferMutex.Lock()
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
	d.bufferMutex.Unlock()
}

func (d *display) cleanUp() {
	d.bufferMutex.Lock()
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
	d.bufferMutex.Unlock()
	d.clear <- struct{}{}
}
