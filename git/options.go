package git

type Options struct {
	Ignore IgnoreOptions `cmd:"" name:"ignore" help:"Download .gitignore from GitHub gitignore templates repo."`
}
