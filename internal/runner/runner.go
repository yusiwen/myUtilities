package runner

import (
	"errors"
	"fmt"
	"os"
	"strings"

	corerunner "github.com/yusiwen/myUtilities/internal/core/runner"
)

func (o *CommandRunnerOptions) Run() error {
	if o.Schema {
		return printSchema()
	}
	if o.File != "" {
		if len(o.Commands) > 0 {
			return errors.New("--file and --command are mutually exclusive")
		}
		return o.runRecipe()
	}
	return o.runInline()
}

func printSchema() error {
	_, err := os.Stdout.Write(recipeSchema)
	if err != nil {
		return err
	}
	fmt.Println()
	return nil
}

func (o *CommandRunnerOptions) runInline() error {
	r := corerunner.NewCommandRunner(o.Commands)
	err := r.Run()
	if errors.Is(err, corerunner.ErrInterrupted) {
		// 128 + SIGINT, the shell convention for an interrupted process.
		os.Exit(130)
	}
	return err
}

func (o *CommandRunnerOptions) runRecipe() error {
	recipe, err := corerunner.LoadRecipe(o.File)
	if err != nil {
		return err
	}
	vars := map[string]string{}
	for _, kv := range o.Var {
		k, v, found := strings.Cut(kv, "=")
		if !found || k == "" {
			return fmt.Errorf("invalid --var %q (expected key=value)", kv)
		}
		vars[k] = v
	}
	var selected []string
	for _, sel := range o.Task {
		selected = append(selected, strings.Split(sel, ",")...)
	}
	r := corerunner.NewCommandRunner(nil)
	_, err = r.RunRecipe(recipe, corerunner.RecipeRunOptions{
		Tasks:     selected,
		Vars:      vars,
		KeepGoing: o.KeepGoing,
		DryRun:    o.DryRun,
	})
	if errors.Is(err, corerunner.ErrInterrupted) {
		os.Exit(130)
	}
	return err
}
