package fleet

// CommonOptions are flags shared by all fleet subcommands.
type CommonOptions struct {
	Config string `help:"Config file path." default:"~/.config/mu/fleet-config.json"`
	Server string `help:"Dispatcher server URL (overrides config)."`
	Token  string `help:"Auth token (overrides config)."`
}

// Options defines the fleet CLI.
type Options struct {
	Serve  ServeCmd  `cmd:"" help:"Start the fleet dispatcher server."`
	Agent  AgentCmd  `cmd:"" help:"Run a fleet agent on this host."`
	Run    RunCmd    `cmd:"" help:"Submit a job to the fleet."`
	Hosts  HostsCmd  `cmd:"" help:"List agents in the fleet."`
	Status StatusCmd `cmd:"" help:"Show a job's status and output."`
	Jobs   JobsCmd   `cmd:"" help:"List recent jobs."`
}

// ServeCmd starts the dispatcher.
type ServeCmd struct {
	CommonOptions
	Port int `help:"Port to listen on (default: config or 8890)." default:"0"`
}

// AgentCmd runs the agent loop.
type AgentCmd struct {
	CommonOptions
	Hostname     string `help:"Agent hostname (default: auto-detect)."`
	Groups       string `help:"Comma-separated groups for this agent."`
	PollInterval int    `help:"Poll interval in seconds (default: config or 5)." default:"0"`
}

// RunCmd submits a job.
type RunCmd struct {
	CommonOptions
	Hosts   string   `help:"Comma-separated target hostnames." required:""`
	File    string   `help:"Recipe file to execute." xor:"file_or_command"`
	Command string   `help:"Command to execute." xor:"file_or_command"`
	Var     []string `help:"Recipe variable in key=value form (repeatable)."`
	Files   []string `help:"File to upload with the job (repeatable)."`
	Watch   bool     `help:"Poll and stream job output until completion."`
}

// HostsCmd lists agents.
type HostsCmd struct {
	CommonOptions
}

// StatusCmd shows job status.
type StatusCmd struct {
	CommonOptions
	JobID string `arg:"" help:"Job ID." required:""`
	Watch bool   `help:"Poll until the job completes."`
}

// JobsCmd lists recent jobs.
type JobsCmd struct {
	CommonOptions
	Limit int `help:"Maximum number of jobs to list." default:"20"`
}
