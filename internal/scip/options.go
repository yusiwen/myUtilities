package scip

type Options struct {
	Install InstallOptions `cmd:"" name:"install" help:"Install an SCIP indexer binary for a language (auto-downloaded like nvim-treesitter parsers)."`
	List    ListOptions    `cmd:"" name:"list" help:"List available and installed indexers."`
	Index   IndexOptions   `cmd:"" name:"index" help:"Generate the SCIP semantic index for the current repository."`
	Update  UpdateOptions  `cmd:"" name:"update" help:"Update indexers to the latest release and persist the version in config."`
	Purge   PurgeOptions   `cmd:"" name:"purge" help:"Remove cached indexers and indexes."`
}

type InstallOptions struct {
	Lang    string `arg:"" help:"Language to install (e.g. go)."`
	Release string `help:"Specific release tag to install (default: configured or pinned version)."`
	Force   bool   `help:"Reinstall even if already present."`
	Token   string `help:"GitHub token." env:"GITHUB_TOKEN"`
}

type ListOptions struct{}

type IndexOptions struct {
	Force    bool   `help:"Regenerate even if a cached index exists."`
	CacheDir string `help:"Cache directory. Default: ~/.cache/mu/scip"`
	Token    string `help:"GitHub token." env:"GITHUB_TOKEN"`
}

type UpdateOptions struct {
	Lang    string `arg:"" optional:"" help:"Language to update (default: all enabled indexers)."`
	All     bool   `help:"Update all enabled indexers (default when no language given)."`
	DryRun  bool   `help:"Show what would be updated without downloading."`
	NoPin   bool   `help:"Update binaries only, do not persist the new version in git-config.json."`
	KeepOld bool   `help:"Keep old installed versions instead of removing them."`
	Token   string `help:"GitHub token." env:"GITHUB_TOKEN"`
}

type PurgeOptions struct {
	Force bool `help:"Skip confirmation." short:"f"`
}
