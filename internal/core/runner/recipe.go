package runner

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

// Recipe is a task-orchestration file loaded from YAML. Tasks are keyed by
// name; dependencies between tasks are expressed through the Depends field.
type Recipe struct {
	Name        string                `yaml:"name"`
	Description string                `yaml:"description"`
	Vars        map[string]string     `yaml:"vars"`
	Env         map[string]string     `yaml:"env"`
	Tasks       map[string]RecipeTask `yaml:"tasks"`
}

// RecipeTask describes a single named task.
type RecipeTask struct {
	Description     string            `yaml:"description"`
	Command         string            `yaml:"command"`
	Commands        []string          `yaml:"commands"`
	Depends         []string          `yaml:"depends"`
	Env             map[string]string `yaml:"env"`
	Workdir         string            `yaml:"workdir"`
	Timeout         string            `yaml:"timeout"` // e.g. "30s"
	Retry           int               `yaml:"retry"`
	ContinueOnError bool              `yaml:"continue_on_error"`
}

// LoadRecipe reads, strictly decodes, and validates a recipe file. Unknown
// fields, unknown dependencies, dependency cycles, missing commands, and
// interactive (!-prefixed) commands are rejected here, before any execution.
func LoadRecipe(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var r Recipe
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("invalid recipe: %w", err)
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Recipe) validate() error {
	if len(r.Tasks) == 0 {
		return fmt.Errorf("recipe has no tasks")
	}
	names := make(map[string]bool, len(r.Tasks))
	for name := range r.Tasks {
		names[name] = true
	}
	for name, task := range r.Tasks {
		if task.Command != "" && len(task.Commands) > 0 {
			return fmt.Errorf("task %q: use either 'command' or 'commands', not both", name)
		}
		if task.Command == "" && len(task.Commands) == 0 {
			return fmt.Errorf("task %q: missing command", name)
		}
		cmds := task.Commands
		if task.Command != "" {
			cmds = []string{task.Command}
		}
		for _, c := range cmds {
			if strings.HasPrefix(c, "!") {
				return fmt.Errorf("task %q: interactive commands are not supported in recipes (found %q)", name, c)
			}
		}
		for _, dep := range task.Depends {
			if !names[dep] {
				return fmt.Errorf("task %q: depends on unknown task %q", name, dep)
			}
		}
		if task.Timeout != "" {
			if _, err := time.ParseDuration(task.Timeout); err != nil {
				return fmt.Errorf("task %q: invalid timeout %q: %w", name, task.Timeout, err)
			}
		}
		if task.Retry < 0 {
			return fmt.Errorf("task %q: retry must not be negative", name)
		}
	}
	// Cycle detection is done by TopologicalOrder.
	if _, err := r.TopologicalOrder(nil); err != nil {
		return err
	}
	return nil
}

// TopologicalOrder returns the task names in dependency order. If selected is
// non-empty, only those tasks and their transitive dependencies are included.
// A dependency cycle is reported as an error.
func (r *Recipe) TopologicalOrder(selected []string) ([]string, error) {
	include := make(map[string]bool)
	if len(selected) == 0 {
		for name := range r.Tasks {
			include[name] = true
		}
	} else {
		var add func(name string)
		add = func(name string) {
			if include[name] {
				return
			}
			if _, ok := r.Tasks[name]; !ok {
				return
			}
			include[name] = true
			for _, dep := range r.Tasks[name].Depends {
				add(dep)
			}
		}
		for _, s := range selected {
			if _, ok := r.Tasks[s]; !ok {
				return nil, fmt.Errorf("unknown task %q", s)
			}
			add(s)
		}
	}

	indeg := make(map[string]int)
	adj := make(map[string][]string)
	for name := range include {
		indeg[name] = 0
	}
	for name := range include {
		for _, dep := range r.Tasks[name].Depends {
			if !include[dep] {
				continue
			}
			adj[dep] = append(adj[dep], name)
			indeg[name]++
		}
	}

	queue := make([]string, 0, len(include))
	for name, d := range indeg {
		if d == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)

	order := make([]string, 0, len(include))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, m := range adj[n] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
		sort.Strings(queue)
	}
	if len(order) != len(include) {
		return nil, fmt.Errorf("dependency cycle detected in recipe tasks")
	}
	return order, nil
}

// renderCommands templates a task's command(s) with the merged variables.
func (r *Recipe) renderCommands(name string, vars map[string]string) ([]string, error) {
	task := r.Tasks[name]
	cmds := task.Commands
	if task.Command != "" {
		cmds = []string{task.Command}
	}
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		rc, err := renderTemplate(c, vars)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", name, err)
		}
		out = append(out, rc)
	}
	return out, nil
}

// renderTemplate expands {{.key}} references using the given variables.
// Unknown variables produce an error (missingkey=error), so a typo'd variable
// is caught instead of silently rendering as empty.
func renderTemplate(s string, vars map[string]string) (string, error) {
	t, err := template.New("var").Option("missingkey=error").Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid template %q: %w", s, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("render template %q: %w", s, err)
	}
	return buf.String(), nil
}

// envSlice converts an environment map into a deterministic KEY=VALUE slice.
func envSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}
