package watch

import (
	"time"
)

type Options struct {
	File FileOptions `cmd:"" name:"file" help:"Watch file/directory for changes."`
	Git  GitOptions  `cmd:"" name:"git" help:"Watch git remote for updates."`
}

type FileOptions struct {
	Dir      string        `arg:"" name:"dir" help:"File or directory to watch."`
	Interval time.Duration `help:"Polling interval." default:"5s"`
	Include  []string      `name:"include" help:"Glob pattern to include (repeatable)."`
	Exclude  []string      `name:"exclude" help:"Glob pattern to exclude (repeatable)."`
}

type GitOptions struct {
	Dir      string        `arg:"" name:"dir" help:"Path to local git repository."`
	Remote   string        `help:"Remote name." default:"origin"`
	Branch   string        `help:"Branch to track." default:""`
	Interval time.Duration `help:"Polling interval." default:"60s"`
}
