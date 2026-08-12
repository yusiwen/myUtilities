package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(data)
}

func recipeWithLogs(dir string) *Recipe {
	log := filepath.Join(dir, "log.txt")
	cmd := func(name string) string { return "echo " + name + " >> " + log }
	return &Recipe{Tasks: map[string]RecipeTask{
		"clean":  {Command: cmd("clean")},
		"build":  {Command: cmd("build"), Depends: []string{"clean"}},
		"deploy": {Command: cmd("deploy"), Depends: []string{"build"}},
	}}
}

func TestRunRecipeSuccess(t *testing.T) {
	dir := t.TempDir()
	r := recipeWithLogs(dir)
	results, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{})
	if err != nil {
		t.Fatalf("RunRecipe: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, res := range results {
		if res.Status != "ok" {
			t.Fatalf("task %q status = %q, want ok", res.Name, res.Status)
		}
	}
	got := readLog(t, filepath.Join(dir, "log.txt"))
	want := "clean\nbuild\ndeploy\n"
	if got != want {
		t.Fatalf("execution order wrong: %q, want %q", got, want)
	}
}

func TestRunRecipeSelectTasks(t *testing.T) {
	dir := t.TempDir()
	r := recipeWithLogs(dir)
	_, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{Tasks: []string{"build"}})
	if err != nil {
		t.Fatalf("RunRecipe: %v", err)
	}
	got := readLog(t, filepath.Join(dir, "log.txt"))
	if got != "clean\nbuild\n" {
		t.Fatalf("expected only clean+build, got %q", got)
	}
}

func TestRunRecipeDryRun(t *testing.T) {
	dir := t.TempDir()
	r := recipeWithLogs(dir)
	if _, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{DryRun: true}); err != nil {
		t.Fatalf("RunRecipe dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "log.txt")); !os.IsNotExist(err) {
		t.Fatal("dry-run should not execute tasks")
	}
}

func TestRunRecipeStopOnFailure(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	r := &Recipe{Tasks: map[string]RecipeTask{
		"bad":     {Command: "echo bad >> " + log + "; exit 1"},
		"z_after": {Command: "echo after >> " + log},
	}}
	results, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{})
	if err == nil {
		t.Fatal("expected aggregate failure")
	}
	if len(results) != 1 || results[0].Name != "bad" {
		t.Fatalf("expected only 'bad' to run, got %+v", results)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error should mention failed task, got %v", err)
	}
}

func TestRunRecipeContinueOnError(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	r := &Recipe{Tasks: map[string]RecipeTask{
		"bad":     {Command: "echo bad >> " + log + "; exit 1", ContinueOnError: true},
		"z_after": {Command: "echo after >> " + log},
	}}
	results, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{})
	if err == nil {
		t.Fatal("expected aggregate failure")
	}
	if len(results) != 2 || results[1].Name != "z_after" || results[1].Status != "ok" {
		t.Fatalf("expected continue after failure, got %+v", results)
	}
	if !strings.Contains(readLog(t, log), "after") {
		t.Fatal("'z_after' task should have run")
	}
}

func TestRunRecipeKeepGoing(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	r := &Recipe{Tasks: map[string]RecipeTask{
		"bad":   {Command: "echo bad >> " + log + "; exit 1"},
		"after": {Command: "echo after >> " + log},
	}}
	results, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{KeepGoing: true})
	if err == nil {
		t.Fatal("expected aggregate failure even with keep-going")
	}
	if len(results) != 2 {
		t.Fatalf("expected both tasks to run, got %+v", results)
	}
	if !strings.Contains(readLog(t, log), "after") {
		t.Fatal("'after' task should have run with --keep-going")
	}
}

func TestRunRecipeRetry(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	r := &Recipe{Tasks: map[string]RecipeTask{
		"flaky": {Command: "echo attempt >> " + log + "; exit 1", Retry: 2},
	}}
	results, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{})
	if err == nil {
		t.Fatal("expected failure after retries exhausted")
	}
	if len(results) != 1 || results[0].Status != "fail" {
		t.Fatalf("unexpected results: %+v", results)
	}
	lines := strings.Count(readLog(t, log), "attempt")
	if lines != 3 {
		t.Fatalf("expected 3 attempts (1 + 2 retries), got %d", lines)
	}
}

func TestRunRecipeTimeout(t *testing.T) {
	start := time.Now()
	r := &Recipe{Tasks: map[string]RecipeTask{
		"slow": {Command: "sleep 5", Timeout: "300ms"},
	}}
	results, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{})
	if err == nil {
		t.Fatal("expected failure")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("timeout not applied, took %v", elapsed)
	}
	if len(results) != 1 || results[0].Status != "fail" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRunRecipeEnvAndVars(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	r := &Recipe{
		Vars: map[string]string{"who": "world"},
		Env:  map[string]string{"GREETING": "hi"},
		Tasks: map[string]RecipeTask{
			"a": {Command: "echo $GREETING {{.who}} >> " + log},
		},
	}
	if _, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{}); err != nil {
		t.Fatalf("RunRecipe: %v", err)
	}
	if got := readLog(t, log); got != "hi world\n" {
		t.Fatalf("env/vars not applied, got %q", got)
	}
}

func TestRunRecipeVarOverride(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log.txt")
	r := &Recipe{
		Vars: map[string]string{"who": "default"},
		Tasks: map[string]RecipeTask{
			"a": {Command: "echo {{.who}} >> " + log},
		},
	}
	if _, err := NewCommandRunner(nil).RunRecipe(r, RecipeRunOptions{Vars: map[string]string{"who": "override"}}); err != nil {
		t.Fatalf("RunRecipe: %v", err)
	}
	if got := readLog(t, log); got != "override\n" {
		t.Fatalf("var override not applied, got %q", got)
	}
}
