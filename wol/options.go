package wol

type ConfigSetOptions struct {
	Key   string `arg:"" help:"Config key (server, interface, db-path, port, token, hostname)."`
	Value string `arg:"" help:"Config value."`
}

type ConfigGetOptions struct {
	Key string `arg:"" help:"Config key to show."`
}

type ConfigListOptions struct{}

type ConfigOptions struct {
	Config string            `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Set    ConfigSetOptions  `cmd:"" name:"set" help:"Set a config value."`
	Get    ConfigGetOptions  `cmd:"" name:"get" help:"Get a config value."`
	List   ConfigListOptions `cmd:"" name:"list" help:"List all config values."`
}

type ServeOptions struct {
	Config    string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Interface string `name:"interface" help:"Network interface name. Overrides config file value."`
	DBPath    string `help:"Override BoltDB file path from config."`
	Port      int    `help:"Override HTTP server port from config."`
	Token     string `help:"Override API auth token from config."`
}

type AgentOptions struct {
	Config    string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Server    string `arg:"" optional:"" help:"WOL HTTP server URL (e.g., http://192.168.1.100:8080). If not set, reads from config file."`
	Hostname  string `help:"Hostname to register on boot. Defaults to OS hostname." default:""`
	Token     string `help:"Override pre-shared token from config. Sent as X-Auth-Token header." default:""`
	Boot      bool   `help:"Notify the server that this machine has booted."`
	Shutdown  bool   `help:"Notify the server that this machine is shutting down."`
	Register  bool   `help:"Register this machine's hostname and MAC on the server."`
	Interface string `name:"interface" help:"Network interface for MAC detection during registration (e.g. en0, eth0)."`
}

type InterfacesOptions struct {
	Verbose bool `help:"Show detailed interface information including IP addresses." short:"V"`
}

type Options struct {
	Config     ConfigOptions     `cmd:"" name:"config" help:"Configure WOL server settings."`
	Serve      ServeOptions      `cmd:"" name:"serve" help:"Start WOL HTTP server."`
	Agent      AgentOptions      `cmd:"" name:"agent" help:"Notify the WOL server of boot or shutdown events."`
	Interfaces InterfacesOptions `cmd:"" name:"interfaces" help:"List available network interfaces."`
}
