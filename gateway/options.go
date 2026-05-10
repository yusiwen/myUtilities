package gateway

type Options struct {
	Port      int    `help:"Gateway HTTP server port." default:"8080"`
	WolConfig string `name:"wol-config" help:"Path to WOL config JSON file." default:"~/.config/mu/wol-config.json"`
	EsConfig  string `name:"es-config" help:"Path to ES config JSON file." default:"~/.config/mu/es-config.json"`
}
