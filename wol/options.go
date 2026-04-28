package wol

type ServeOptions struct {
	Interface string `arg:"" help:"Network interface name (e.g., br-lan on Linux, en0 on macOS, Ethernet0 on Windows). Use 'mu wol interfaces' to list available interfaces."`
	DBPath    string `help:"Path to BoltDB file storing hostname to MAC mappings." default:"~/.config/go-wol/bolt.db"`
	Port      int    `help:"HTTP server port." default:"8080"`
}

type AgentOptions struct {
	Server   string `arg:"" help:"WOL HTTP server URL (e.g., http://192.168.1.100:8080)."`
	Hostname string `help:"Hostname to register on boot. Defaults to OS hostname." default:""`
	Boot     bool   `help:"Notify the server that this machine has booted."`
	Shutdown bool   `help:"Notify the server that this machine is shutting down."`
	Register bool   `help:"Register this machine's hostname and MAC on the server."`
}

type InterfacesOptions struct {
	Verbose bool `help:"Show detailed interface information including IP addresses." short:"V"`
}

type Options struct {
	Serve      ServeOptions      `cmd:"" name:"serve" help:"Start WOL HTTP server."`
	Agent      AgentOptions      `cmd:"" name:"agent" help:"Notify the WOL server of boot or shutdown events."`
	Interfaces InterfacesOptions `cmd:"" name:"interfaces" help:"List available network interfaces."`
}
