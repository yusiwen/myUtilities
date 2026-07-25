package git

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/core/config"
)

type commitSetter struct{}

func init() {
	config.Register(&commitSetter{})
}

func (s *commitSetter) Name() string {
	return "commit"
}

func (s *commitSetter) Set(args []string) error {
	opts := &SetOptions{}
	parser, err := kong.New(opts, kong.Name("mu set commit"), kong.Description("Update git commit config."))
	if err != nil {
		return err
	}
	_, err = parser.Parse(args)
	if err != nil {
		return err
	}
	return opts.Run()
}
