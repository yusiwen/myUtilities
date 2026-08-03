package gateway

type Options struct {
	Port         int    `help:"Gateway HTTP server port." default:"8080"`
	ConfigDir    string `name:"config-dir" help:"Directory to read module configs from." default:"~/.config/mu"`
	SvcregServer string `name:"svcreg-server" help:"Service registry server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
}
