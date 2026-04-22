package wol

type Options struct {
	Interface string `arg:"" help:"Network interface name (e.g., br-lan)."`
	DBPath    string `help:"Path to BoltDB file storing hostname to MAC mappings." default:"~/.config/go-wol/bolt.db"`
	Port      int    `help:"HTTP server port." default:"8080"`
}
