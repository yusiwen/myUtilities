package runner

import (
	"github.com/yusiwen/myUtilities/core/runner"
)

type CommandRunnerOptions struct {
	Commands []runner.Command `embed:"" prefix:"runner." help:"Commands to run."`
}
