package git

type Options struct {
	Ignore IgnoreOptions `cmd:"" name:"ignore" help:"Download .gitignore from GitHub gitignore templates repo."`
	Commit CommitOptions `cmd:"" name:"commit" help:"Generate conventional commit message using AI."`
	Review ReviewOptions `cmd:"" name:"review" help:"AI-powered code review for local changes."`
}

type ReviewOptions struct {
	Model       string   `help:"Model name to use." short:"m" env:"OPENAI_MODEL"`
	APIKey      string   `help:"API key for the AI service." short:"k" env:"OPENAI_API_KEY"`
	BaseURL     string   `help:"Base URL of the AI service." short:"u" env:"OPENAI_BASE_URL"`
	Staged      bool     `help:"Review staged changes (git diff --staged)." short:"s"`
	Base        string   `help:"Base branch or commit for comparison. E.g. origin/main" short:"b"`
	Target      string   `help:"Target branch or commit. Default: HEAD" short:"t"`
	Context     string   `help:"Additional context for the reviewer (e.g. 'focus on security')." short:"C"`
	Lang        string   `help:"Review output language." short:"L" default:"en" enum:"en,cn"`
	Verbose     bool     `help:"Print prompts and raw API responses for debugging."`
	MaxTurns    int      `help:"Maximum number of tool call rounds." default:"20"`
	NoScip      bool     `help:"Disable SCIP semantic code intelligence tools."`
	RefreshScip bool     `help:"Force regeneration of the SCIP index."`
	Paths       []string `arg:"" optional:"" name:"path" help:"Files or paths to review."`
	List        bool     `help:"List saved review reports." name:"list"`
	ListAll     bool     `help:"When listing, include reviews from all projects." name:"list-all"`
}

type ReviewListCmd struct {
	All bool `help:"List reviews from all projects, not just current." name:"all"`
}
