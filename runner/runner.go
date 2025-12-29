package runner

import (
	"fmt"

	"github.com/yusiwen/myUtilities/core/runner"
)

func (o *CommandRunnerOptions) Run() error {

	fmt.Println(o.Commands)

	r := runner.NewCommandRunner(o.Commands)
	r.Run()

	return nil
}
