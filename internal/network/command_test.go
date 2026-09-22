package network

import (
	"testing"

	"github.com/alecthomas/kong"
)

func parseNetwork(t *testing.T, args ...string) (*kong.Context, *Options) {
	t.Helper()
	root := struct {
		Network Options `cmd:"" name:"network"`
	}{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return ctx, &root.Network
}

// TestBareInvocationSelectsDefaultCommand pins the Kong wiring that makes the
// `--server` shortcut reachable: without the hidden default subcommand, Kong
// rejects `mu network` with "expected one of ..." before Options.Run runs.
func TestBareInvocationSelectsDefaultCommand(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"network"}, bareCmdName},
		{[]string{"network", "--server"}, bareCmdName},
		{[]string{"network", "serve"}, "serve"},
		{[]string{"network", "dns", "example.com"}, "dns"},
		{[]string{"network", "port-scan"}, "port-scan"},
	}
	for _, tc := range tests {
		ctx, _ := parseNetwork(t, tc.args...)
		selected := ctx.Selected()
		if selected == nil {
			t.Fatalf("parse %v: nothing selected", tc.args)
		}
		if selected.Name != tc.want {
			t.Errorf("parse %v: selected %q, want %q", tc.args, selected.Name, tc.want)
		}
	}
}

// TestServerShortcutParsesOnBareCommand verifies the --server flag is still
// populated on the parent for the bare invocation, which is what Options.Run
// reads to start the server.
func TestServerShortcutParsesOnBareCommand(t *testing.T) {
	_, withFlag := parseNetwork(t, "network", "--server")
	if !withFlag.Server {
		t.Error("--server was not parsed on the parent command")
	}

	_, withoutFlag := parseNetwork(t, "network")
	if withoutFlag.Server {
		t.Error("--server should default to false")
	}
}
