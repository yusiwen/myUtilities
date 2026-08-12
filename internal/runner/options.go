package runner

import (
	"github.com/yusiwen/myUtilities/internal/core/runner"
)

type CommandRunnerOptions struct {
	Commands  []runner.Command `name:"command" help:"Shell command to run. Repeat to run multiple. Format: [<name>::][!]<command> (prefix ! for interactive, e.g. !sudo whoami). Mutually exclusive with --file."`
	File      string           `short:"f" name:"file" help:"Recipe file (YAML) with named tasks, dependencies, and variables."`
	Task      []string         `name:"task" help:"Run only these tasks and their dependencies (repeatable or comma-separated)."`
	Var       []string         `name:"var" help:"Override a recipe variable (key=value, repeatable)."`
	KeepGoing bool             `name:"keep-going" help:"Continue running tasks after a failure."`
	DryRun    bool             `name:"dry-run" help:"Print the ordered task list without executing."`
	Schema    bool             `name:"schema" help:"Print the recipe JSON Schema (for editor validation) and exit."`
}
