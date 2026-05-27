package wol

type SetDBPathOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	DBPath string `arg:"" help:"Path to BoltDB file storing hostname to MAC mappings."`
}

type SetPortOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Port   int    `arg:"" help:"HTTP server port for the WOL server."`
}

type SetTokenOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Token  string `arg:"" help:"Pre-shared token for API authentication. Agents and frontend must send X-Auth-Token header."`
}

type SetInterfaceOptions struct {
	Config    string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Interface string `arg:"" help:"Network interface name for WOL (e.g., br-lan on Linux, en0 on macOS)."`
}

type SetHostnameOptions struct {
	Config   string `help:"Path to config JSON file." default:"~/.config/mu/wol-config.json"`
	Hostname string `arg:"" help:"Hostname used by agent for registration. Defaults to OS hostname if not set."`
}

type SetOptions struct {
	Interface SetInterfaceOptions `cmd:"" name:"interface" help:"Set the network interface for the WOL server."`
	DBPath    SetDBPathOptions    `cmd:"" name:"db-path" help:"Set the BoltDB file path for the WOL server."`
	Port      SetPortOptions      `cmd:"" name:"port" help:"Set the HTTP server port for the WOL server."`
	Token     SetTokenOptions     `cmd:"" name:"token" help:"Set the pre-shared API token for the WOL server."`
	Hostname  SetHostnameOptions  `cmd:"" name:"hostname" help:"Set the hostname for agent registration."`
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
	Server    string `arg:"" help:"WOL HTTP server URL (e.g., http://192.168.1.100:8080)."`
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
	Set        SetOptions        `cmd:"" name:"set" help:"Configure WOL server settings."`
	Serve      ServeOptions      `cmd:"" name:"serve" help:"Start WOL HTTP server."`
	Agent      AgentOptions      `cmd:"" name:"agent" help:"Notify the WOL server of boot or shutdown events."`
	Interfaces InterfacesOptions `cmd:"" name:"interfaces" help:"List available network interfaces."`
}
