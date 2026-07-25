package ask

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
)

type askSetter struct{}

func init() {
	config.Register(&askSetter{})
}

func (s *askSetter) Name() string {
	return "ask"
}

func (s *askSetter) Set(args []string) error {
	opts := &SetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set ask"), kong.Description("Update ask config."))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	if err != nil {
		return err
	}
	return opts.Run()
}
