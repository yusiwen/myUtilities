package runner

import (
	"github.com/yusiwen/myUtilities/internal/core/runner"
)

type CommandRunnerOptions struct {
	Commands []runner.Command `name:"command" help:"Shell command to run. Repeat to run multiple. Format: <name>::<command>."`
}
