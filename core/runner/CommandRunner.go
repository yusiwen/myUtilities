package runner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
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

func (r *CommandRunner) Run() {
	go r.runCommands()
	go r.d.refreshBuffer()
	go r.d.update()

	r.wg.Wait()
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
	//time.Sleep(1 * time.Second)

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

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		r.output <- scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Output reading error: %v", err)
	}

	var errorMsg string
	if errMsgBytes, err := io.ReadAll(stderr); err == nil {
		errorMsg = string(errMsgBytes)
	} else {
		log.Printf("Failed to read stderr: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			r.done <- &CmdStatus{
				isSuccess: false,
				exitCode:  exitError.ExitCode(),
				errMsg:    errorMsg,
			}
			return errors.New(errorMsg)
		} else {
			log.Printf("cmd.Wait() error: %v", err)
		}
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

	wg *sync.WaitGroup
	d  *display
}

func NewCommandRunner(commands []Command) *CommandRunner {
	output := make(chan string)
	done := make(chan *CmdStatus)
	clearDone := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(3)

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
}

func (d *display) update() {
	defer d.wg.Done()
	defer close(d.clear)

	for {
		select {
		case <-d.ticker.C:
			d.print()
		case status := <-d.done:
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
		fmt.Println(ANSI_CLEAR_LINE, out)
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
		d.prevLines = 0
		d.buffer = nil
		d.isHidden = true
	}
	d.bufferMutex.Unlock()
	d.clear <- struct{}{}
}

func main() {
}
