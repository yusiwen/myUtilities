package runner

import (
	"github.com/yusiwen/myUtilities/core/runner"
)

func (o *CommandRunnerOptions) Run() error {
	r := runner.NewCommandRunner(o.Commands)
	return r.Run()
}
