package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLoadRecipeSuccess(t *testing.T) {
	p := writeTemp(t, "r.yaml", `
name: demo
vars:
  v: "1"
env:
  K: V
tasks:
  a:
    command: echo hi
  b:
    commands:
      - echo x
      - echo y
    depends: [a]
    timeout: 30s
    retry: 2
    continue_on_error: true
`)
	r, err := LoadRecipe(p)
	if err != nil {
		t.Fatalf("LoadRecipe: %v", err)
	}
	if r.Name != "demo" || r.Vars["v"] != "1" || r.Env["K"] != "V" {
		t.Fatalf("recipe fields not loaded: %+v", r)
	}
	if len(r.Tasks["b"].Commands) != 2 {
		t.Fatalf("expected 2 commands, got %v", r.Tasks["b"].Commands)
	}
}

func TestLoadRecipeValidation(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"no tasks", `name: x`, "no tasks"},
		{"unknown field", "tasks:\n  a:\n    command: echo hi\n    bogus: 1\n", "field bogus not found"},
		{"missing command", "tasks:\n  a:\n    depends: []\n", "missing command"},
		{"command and commands", "tasks:\n  a:\n    command: echo hi\n    commands: [echo x]\n", "not both"},
		{"interactive", "tasks:\n  a:\n    command: \"!sudo whoami\"\n", "interactive commands are not supported"},
		{"unknown depends", "tasks:\n  a:\n    command: echo hi\n    depends: [nope]\n", "unknown task \"nope\""},
		{"bad timeout", "tasks:\n  a:\n    command: echo hi\n    timeout: fast\n", "invalid timeout"},
		{"negative retry", "tasks:\n  a:\n    command: echo hi\n    retry: -1\n", "retry must not be negative"},
		{"cycle", "tasks:\n  a:\n    command: echo a\n    depends: [b]\n  b:\n    command: echo b\n    depends: [a]\n", "dependency cycle"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := writeTemp(t, "bad.yaml", c.yaml)
			_, err := LoadRecipe(p)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err, c.want)
			}
		})
	}
}

func TestTopologicalOrder(t *testing.T) {
	r := &Recipe{Tasks: map[string]RecipeTask{
		"clean":   {},
		"build":   {Depends: []string{"clean"}},
		"test":    {Depends: []string{"build"}},
		"deploy":  {Depends: []string{"build"}},
		"release": {Depends: []string{"test", "deploy"}},
	}}

	order, err := r.TopologicalOrder(nil)
	if err != nil {
		t.Fatalf("TopologicalOrder: %v", err)
	}
	// deps must precede dependents
	idx := map[string]int{}
	for i, n := range order {
		idx[n] = i
	}
	if idx["clean"] > idx["build"] || idx["build"] > idx["test"] ||
		idx["build"] > idx["deploy"] || idx["test"] > idx["release"] || idx["deploy"] > idx["release"] {
		t.Fatalf("invalid order: %v", order)
	}

	// selected subset pulls in transitive dependencies
	sub, err := r.TopologicalOrder([]string{"deploy"})
	if err != nil {
		t.Fatalf("TopologicalOrder(selected): %v", err)
	}
	if !reflect.DeepEqual(sub, []string{"clean", "build", "deploy"}) {
		t.Fatalf("expected [clean build deploy], got %v", sub)
	}

	if _, err := r.TopologicalOrder([]string{"missing"}); err == nil {
		t.Fatal("expected error for unknown selected task")
	}
}

func TestRenderTemplate(t *testing.T) {
	out, err := renderTemplate("build {{.version}} for {{.os}}", map[string]string{"version": "1.2.3", "os": "linux"})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if out != "build 1.2.3 for linux" {
		t.Fatalf("got %q", out)
	}
	if _, err := renderTemplate("{{.missing}}", map[string]string{}); err == nil {
		t.Fatal("expected error for unknown variable")
	}
}

func TestEnvSliceDeterministic(t *testing.T) {
	got := envSlice(map[string]string{"B": "2", "A": "1", "C": "3"})
	want := []string{"A=1", "B=2", "C=3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("envSlice = %v, want %v", got, want)
	}
}
