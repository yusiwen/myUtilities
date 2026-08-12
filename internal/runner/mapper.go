package runner

import (
	"fmt"
	"reflect"

	"github.com/alecthomas/kong"
	corerunner "github.com/yusiwen/myUtilities/internal/core/runner"
)

// commandMapper decodes a single --command value into a core runner.Command.
// The accepted format is "[<name>::][!]<command line>": an optional name
// separated by "::", and an optional leading "!" on the command line marking
// the command as interactive (stdin/stdout/stderr wired to the terminal).
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
	target.Set(reflect.ValueOf(corerunner.ParseCommandSpec(val)))
	return nil
}

// TypeMapperOption returns the kong option that wires the --command mapper so
// repeatable structured command flags work.
func TypeMapperOption() kong.Option {
	return kong.TypeMapper(reflect.TypeOf(corerunner.Command{}), commandMapper{})
}
