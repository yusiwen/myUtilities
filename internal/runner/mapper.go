package runner

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
	corerunner "github.com/yusiwen/myUtilities/internal/core/runner"
)

// commandMapper decodes a single --command value into a core runner.Command.
// The accepted format is "<name>::<command line>"; when the separator is
// absent the whole value is treated as the command line with no name.
type commandMapper struct{}

func (commandMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	t := ctx.Scan.Pop()
	if t.IsEOL() {
		return fmt.Errorf("missing value for --command")
	}
	val, ok := t.Value.(string)
	if !ok {
		return fmt.Errorf("invalid value for --command: %v", t.Value)
	}
	var c corerunner.Command
	if name, cmd, found := strings.Cut(val, "::"); found {
		c.Name = name
		c.CmdLine = cmd
	} else {
		c.CmdLine = val
	}
	target.Set(reflect.ValueOf(c))
	return nil
}

// TypeMapperOption returns the kong option that wires the --command mapper so
// repeatable structured command flags work.
func TypeMapperOption() kong.Option {
	return kong.TypeMapper(reflect.TypeOf(corerunner.Command{}), commandMapper{})
}
