package runner

import (
	"testing"

	"github.com/alecthomas/kong"
	corerunner "github.com/yusiwen/myUtilities/internal/core/runner"
)

func TestCommandMapper(t *testing.T) {
	var cli struct {
		Commands []corerunner.Command `name:"command"`
	}
	parser, err := kong.New(&cli, TypeMapperOption())
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, err = parser.Parse([]string{
		"--command", "greet::echo hello",
		"--command", "ls -la",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cli.Commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(cli.Commands))
	}
	first, second := cli.Commands[0], cli.Commands[1]
	if first.Name != "greet" || first.CmdLine != "echo hello" {
		t.Fatalf("first command = %+v, want name=greet cmd=echo hello", first)
	}
	if second.Name != "" || second.CmdLine != "ls -la" {
		t.Fatalf("second command = %+v, want bare cmdline ls -la", second)
	}
}

func TestCommandMapperMissingValue(t *testing.T) {
	var cli struct {
		Commands []corerunner.Command `name:"command"`
	}
	parser, err := kong.New(&cli, TypeMapperOption())
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse([]string{"--command"}); err == nil {
		t.Fatal("expected error for missing --command value")
	}
}
