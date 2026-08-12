package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/morikuni/aec"
)

// RecipeRunOptions controls how a recipe is executed.
type RecipeRunOptions struct {
	Tasks     []string          // selected tasks (empty = all)
	Vars      map[string]string // CLI variable overrides
	KeepGoing bool
	DryRun    bool
}

// TaskResult records the outcome of one task for the final summary.
type TaskResult struct {
	Name     string
	Status   string // "ok" or "fail"
	Duration time.Duration
}

// RunRecipe executes the tasks of a recipe in dependency order, reusing the
// live display for non-interactive output. It returns the per-task results and
// a non-nil error if any task failed (unless KeepGoing, in which case it still
// reports the failures) or if execution was interrupted (ErrInterrupted).
func (r *CommandRunner) RunRecipe(recipe *Recipe, opts RecipeRunOptions) ([]TaskResult, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		r.interrupted.Store(true)
		r.signalActive(os.Interrupt)
	}()

	results, err := r.runRecipe(recipe, opts)
	if r.interrupted.Load() {
		return results, ErrInterrupted
	}
	return results, err
}

func (r *CommandRunner) runRecipe(recipe *Recipe, opts RecipeRunOptions) ([]TaskResult, error) {
	order, err := recipe.TopologicalOrder(opts.Tasks)
	if err != nil {
		return nil, err
	}

	vars := make(map[string]string, len(recipe.Vars)+len(opts.Vars))
	for k, v := range recipe.Vars {
		vars[k] = v
	}
	for k, v := range opts.Vars {
		vars[k] = v
	}

	type prepared struct {
		name    string
		cmds    []Command
		timeout time.Duration
		retry   int
		contErr bool
	}
	preparedTasks := make([]prepared, 0, len(order))
	for _, name := range order {
		task := recipe.Tasks[name]
		cmds, err := recipe.renderCommands(name, vars)
		if err != nil {
			return nil, err
		}
		var timeout time.Duration
		if task.Timeout != "" {
			timeout, err = time.ParseDuration(task.Timeout)
			if err != nil {
				return nil, fmt.Errorf("task %q: invalid timeout %q: %w", name, task.Timeout, err)
			}
		}
		workdir, err := renderTemplate(task.Workdir, vars)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", name, err)
		}
		env := append(envSlice(recipe.Env), envSlice(task.Env)...)
		cmdList := make([]Command, 0, len(cmds))
		for _, c := range cmds {
			cmdList = append(cmdList, Command{Name: name, CmdLine: c, Env: env, Dir: workdir, Timeout: timeout})
		}
		preparedTasks = append(preparedTasks, prepared{
			name:    name,
			cmds:    cmdList,
			timeout: timeout,
			retry:   task.Retry,
			contErr: task.ContinueOnError,
		})
	}

	if opts.DryRun {
		for _, p := range preparedTasks {
			fmt.Println(p.name)
		}
		return nil, nil
	}

	r.usePTY = isTerminal(os.Stdout) && runtime.GOOS != "windows"
	useDisplay := isTerminal(os.Stdout)
	if useDisplay {
		r.d.width = termWidth()
	}

	var results []TaskResult

	runLoop := func() error {
		var failures []string
		for _, p := range preparedTasks {
			if r.interrupted.Load() {
				break
			}
			start := time.Now()
			taskErr := r.runTask(p.name, p.cmds, p.timeout, useDisplay)
			elapsed := time.Since(start)
			for attempt := 1; taskErr != nil && attempt <= p.retry && !r.interrupted.Load(); attempt++ {
				fmt.Println(aec.Apply(fmt.Sprintf("task %q failed, retrying %d/%d...", p.name, attempt, p.retry), aec.Faint))
				start = time.Now()
				taskErr = r.runTask(p.name, p.cmds, p.timeout, useDisplay)
				elapsed = time.Since(start)
			}
			if r.interrupted.Load() {
				break
			}
			if taskErr != nil {
				failures = append(failures, p.name)
				results = append(results, TaskResult{Name: p.name, Status: "fail", Duration: elapsed})
				fmt.Println(aec.Apply(fmt.Sprintf("✗ task %s failed after %s", p.name, formatElapsed(elapsed)), errColor))
				if !p.contErr && !opts.KeepGoing {
					break
				}
				continue
			}
			results = append(results, TaskResult{Name: p.name, Status: "ok", Duration: elapsed})
		}
		if len(failures) > 0 {
			return fmt.Errorf("%d task(s) failed: %s", len(failures), strings.Join(failures, ", "))
		}
		return nil
	}

	var loopErr error
	if useDisplay {
		r.wg.Add(2)
		go r.d.feedOutput()
		go r.d.update()
		loopErr = runLoop()
		close(r.output)
		close(r.done)
		r.wg.Wait()
	} else {
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
		loopErr = runLoop()
		close(r.output)
		close(r.done)
		<-printerDone
		<-statusDone
	}

	if r.interrupted.Load() {
		fmt.Println("Interrupted.")
	} else {
		printSummary(results)
	}
	return results, loopErr
}

// runTask runs the commands of one task sequentially, each rendered as a live
// display step (TTY mode) or a plain Executing line (piped mode). It returns
// the first error encountered, or ErrInterrupted.
func (r *CommandRunner) runTask(name string, cmds []Command, timeout time.Duration, useDisplay bool) error {
	for i, cmd := range cmds {
		if r.interrupted.Load() {
			return ErrInterrupted
		}
		var start time.Time
		if useDisplay {
			r.d.startStep(name)
		} else {
			fmt.Printf("Executing [%s]...\n", name)
			start = time.Now()
		}
		err := r.runCommand(cmd)
		if useDisplay {
			<-r.d.clear
			start = r.d.stepStart
		}
		elapsed := time.Since(start)
		if err != nil {
			if r.interrupted.Load() {
				return ErrInterrupted
			}
			if errors.Is(err, context.DeadlineExceeded) {
				err = fmt.Errorf("timed out after %s", timeout)
			}
			if useDisplay {
				fmt.Println(aec.Apply(fmt.Sprintf("Executing [%s]... ✗ %s", name, formatElapsed(elapsed)), errColor))
				for _, l := range r.d.failedOut {
					fmt.Println(l)
				}
			}
			fmt.Println(aec.Apply("Error: "+err.Error(), errColor))
			return err
		}
		if useDisplay {
			suffix := ""
			if len(cmds) > 1 {
				suffix = fmt.Sprintf(" (%d/%d)", i+1, len(cmds))
			}
			fmt.Println(aec.Apply(fmt.Sprintf("Executing [%s]... ✓ %s%s", name, formatElapsed(elapsed), suffix), successColor))
		} else {
			fmt.Printf("%s ✓ (%s)\n", name, formatElapsed(elapsed))
		}
	}
	return nil
}

func printSummary(results []TaskResult) {
	if len(results) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Recipe summary:")
	width := 0
	for _, res := range results {
		if len(res.Name) > width {
			width = len(res.Name)
		}
	}
	for _, res := range results {
		mark := "✓"
		color := successColor
		if res.Status == "fail" {
			mark = "✗"
			color = errColor
		}
		fmt.Printf("  %-*s  %s %s\n", width, res.Name, aec.Apply(mark, color), formatElapsed(res.Duration))
	}
}
