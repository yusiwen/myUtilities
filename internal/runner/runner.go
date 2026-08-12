package runner

import (
	"errors"
	"os"

	"github.com/yusiwen/myUtilities/internal/core/runner"
)

func (o *CommandRunnerOptions) Run() error {
	r := runner.NewCommandRunner(o.Commands)
	err := r.Run()
	if errors.Is(err, runner.ErrInterrupted) {
		// 128 + SIGINT, the shell convention for an interrupted process.
		os.Exit(130)
	}
	return err
}
