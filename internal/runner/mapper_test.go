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
	if first.Name != "greet" || first.CmdLine != "echo hello" || first.Interactive {
		t.Fatalf("first command = %+v, want name=greet cmd=echo hello non-interactive", first)
	}
	if second.Name != "" || second.CmdLine != "ls -la" || second.Interactive {
		t.Fatalf("second command = %+v, want bare cmdline ls -la", second)
	}
}

func TestCommandMapperInteractive(t *testing.T) {
	var cli struct {
		Commands []corerunner.Command `name:"command"`
	}
	parser, err := kong.New(&cli, TypeMapperOption())
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, err = parser.Parse([]string{
		"--command", "!sudo whoami",
		"--command", "install::!apt install foo",
		"--command", "echo plain",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cli.Commands) != 3 {
		t.Fatalf("got %d commands, want 3", len(cli.Commands))
	}
	bare, named, plain := cli.Commands[0], cli.Commands[1], cli.Commands[2]
	if !bare.Interactive || bare.CmdLine != "sudo whoami" || bare.Name != "" {
		t.Fatalf("bare interactive = %+v, want !stripped cmd=sudo whoami", bare)
	}
	if !named.Interactive || named.CmdLine != "apt install foo" || named.Name != "install" {
		t.Fatalf("named interactive = %+v, want name=install cmd=apt install foo", named)
	}
	if plain.Interactive || plain.CmdLine != "echo plain" {
		t.Fatalf("plain command = %+v, want non-interactive", plain)
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
