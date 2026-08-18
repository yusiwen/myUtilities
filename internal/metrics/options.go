package metrics

type Options struct {
	Serve   ServeOptions   `cmd:"" name:"serve" help:"Start metrics HTTP server (optionally with built-in agent)."`
	Agent   AgentOptions   `cmd:"" name:"agent" help:"Start metrics collection agent."`
	Compact CompactOptions `cmd:"" name:"compact" help:"Manually compact/expire old data."`
	Query   QueryOptions   `cmd:"" name:"query" help:"Query stored metrics."`
	Status  StatusOptions  `cmd:"" name:"status" help:"Print configuration and running state."`
}

type ServeOptions struct {
	Port      int    `help:"HTTP API port." default:"8096"`
	ConfigDir string `name:"config-dir" help:"Directory for metrics-config.json and the default DB directory. Default: ~/.config/mu."`
	DBPath    string `name:"db-path" help:"Override BoltDB file path. Default: <config-dir>/metrics.db, else ~/.local/share/mu/metrics/metrics.db."`
	Retention string `help:"Data retention (e.g. 30d, 7d, 0=forever, default 0). Empty inherits the config file."`
	Agent     bool   `help:"Also run agent locally."`
	Interval  string `help:"Collect interval (only with --agent)." default:"30s"`
	Hostname  string `help:"Override hostname for tags."`
	Debug     bool   `help:"Enable debug logging."`
}

type AgentOptions struct {
	Server    string `help:"Metrics server URL to report to."`
	ConfigDir string `name:"config-dir" help:"Directory for metrics-config.json and the default DB directory. Default: ~/.config/mu."`
	DBPath    string `name:"db-path" help:"Override BoltDB file path. Default: <config-dir>/metrics.db, else ~/.local/share/mu/metrics/metrics.db."`
	Interval  string `help:"Collect interval." default:"30s"`
	Hostname  string `help:"Override hostname for tags."`
	Retention string `help:"Local data retention (when no server). Default 0=forever. Empty inherits the config file."`
	Debug     bool   `help:"Enable debug logging."`
}

type StatusOptions struct {
	ConfigDir string `name:"config-dir" help:"Directory to look for the metrics Unix sockets. Default: ~/.config/mu."`
	Port      int    `help:"Metrics server port to check (HTTP fallback)." default:"8096"`
	Server    string `help:"Remote metrics server URL to check over HTTP instead of local sockets."`
}

type CompactOptions struct {
	Server    string `help:"Metrics server URL." default:"http://localhost:8096"`
	Retention string `help:"Data retention (e.g. 30d, 7d, 0=forever)." default:"30d"`
}

type QueryOptions struct {
	Name   string `arg:"" optional:"" help:"Metric name to query."`
	Server string `help:"Metrics server URL." default:"http://localhost:8096"`
	Last   string `help:"Time range shortcut (e.g. 1h, 30m)." default:"10m"`
	From   string `help:"Start time (RFC3339)."`
	To     string `help:"End time (RFC3339)." default:"now"`
	Tags   string `help:"Tag filter (k=v,k=v)."`
	Limit  int    `help:"Max data points per series." default:"100"`
	Format string `help:"Output format: table, json, csv." default:"table" enum:"table,json,csv"`
	List   bool   `help:"List all metric names."`
}
