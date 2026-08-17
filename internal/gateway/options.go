package gateway

type Options struct {
	Port             int    `help:"Gateway HTTP server port." default:"8080"`
	ConfigDir        string `name:"config-dir" help:"Directory to read module configs from." default:"~/.config/mu"`
	SvcregServer     string `name:"svcreg-server" help:"Service registry server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
	MetricsServer    string `name:"metrics-server" help:"Metrics server address (read-only proxy)." default:"http://localhost:8096" env:"MU_METRICS_SERVER"`
	MetricsManage    bool   `name:"metrics-manage" help:"Manage a mu metrics serve subprocess from the gateway." default:"true"`
	MetricsPort      int    `name:"metrics-port" help:"Port for the managed metrics server." default:"8096"`
	MetricsAutoStart bool   `name:"metrics-auto-start" help:"Auto-start the managed metrics server when the gateway starts." default:"true"`
}
