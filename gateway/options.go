package gateway

type Options struct {
	Port        int    `help:"Gateway HTTP server port." default:"8080"`
	WolConfig   string `name:"wol-config" help:"Path to WOL config JSON file." default:"~/.config/mu/wol-config.json"`
	EsConfig    string `name:"es-config" help:"Path to ES config JSON file." default:"~/.config/mu/es-config.json"`
	MockConfig  string `name:"mock-config" help:"Path to mock dynamic server config JSON file." default:"~/.config/mu/mock-config.json"`
	SvcregServer string `name:"svcreg-server" help:"Service registry server address for the frontend." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
}
