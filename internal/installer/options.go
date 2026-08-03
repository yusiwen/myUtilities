package installer

type Options struct {
	Repo string `arg:"" help:"GitHub repository."`

	Output    string `help:"Output format, can be 'shell', 'json'" default:"shell" short:"o"`
	Token     string `help:"GitHub token." short:"t" env:"GITHUB_TOKEN"`
	Insecure  bool   `help:"Allow insecure connections." short:"k"`
	AsProgram string `help:"Install as different name."`
	Select    string `help:"Select from list of available releases."`
	Os        string `help:"Install for different OS."`
	Arch      string `help:"Install for different architecture."`
	Move      bool   `help:"Move binary to /usr/local/bin."`
}
