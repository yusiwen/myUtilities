package es

type SetHostOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/es-config.json"`
	Host   string `arg:"" help:"Elasticsearch server URL (e.g., http://localhost:9200)."`
}

type SetUserOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/es-config.json"`
	User   string `arg:"" help:"Elasticsearch username for basic auth."`
}

type SetPasswordOptions struct {
	Config   string `help:"Path to config JSON file." default:"~/.config/mu/es-config.json"`
	Password string `arg:"" help:"Elasticsearch password for basic auth."`
}

type SetOptions struct {
	Host     SetHostOptions     `cmd:"" name:"host" help:"Set the Elasticsearch server URL."`
	User     SetUserOptions     `cmd:"" name:"user" help:"Set the Elasticsearch username."`
	Password SetPasswordOptions `cmd:"" name:"password" help:"Set the Elasticsearch password."`
}

type ServeOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/es-config.json"`
	Host   string `help:"Host to listen on (default 127.0.0.1)."`
	Port   int    `help:"HTTP server port." default:"8084"`
}

type Options struct {
	Set   SetOptions   `cmd:"" name:"set" help:"Configure Elasticsearch connection settings."`
	Serve ServeOptions `cmd:"" name:"serve" help:"Start the Elasticsearch web UI."`
}
