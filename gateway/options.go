package gateway

type Options struct {
	Port         int    `help:"Gateway HTTP server port." default:"8080"`
	WolInterface string `name:"wol-interface" help:"Network interface name for WOL (e.g., br-lan)."`
	WolDB        string `name:"wol-db" help:"Path to BoltDB file for WOL." default:"~/.config/go-wol/bolt.db"`
	WolToken     string `name:"wol-token" help:"Pre-shared token for WOL API authentication."`
	EsConfig     string `name:"es-config" help:"Path to ES config JSON file." default:"~/.config/mu/es-config.json"`
}
