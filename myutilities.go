package main

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/commit"
	"github.com/yusiwen/myUtilities/completion"
	"github.com/yusiwen/myUtilities/crypto"
	"github.com/yusiwen/myUtilities/es"
	"github.com/yusiwen/myUtilities/gateway"
	"github.com/yusiwen/myUtilities/git"
	"github.com/yusiwen/myUtilities/installer"
	"github.com/yusiwen/myUtilities/mock"
	"github.com/yusiwen/myUtilities/proxy"
	"github.com/yusiwen/myUtilities/runner"
	"github.com/yusiwen/myUtilities/wol"
)

type MyUtilities struct {
	Version    kong.VersionFlag            `short:"v" help:"Print the version number"`
	Installer  installer.Options           `cmd:"" name:"install" help:"Install binary from GitHub release."`
	Mocker     mock.Options                `cmd:"" name:"mock" help:"Mockers."`
	Proxy      proxy.Options               `cmd:"" name:"proxy" help:"Proxies."`
	Runner     runner.CommandRunnerOptions `cmd:"" name:"run" help:"Run commands."`
	Wol        wol.Options                 `cmd:"" name:"wol" help:"Wake-on-Lan HTTP server."`
	Es         es.Options                  `cmd:"" name:"es" help:"Elasticsearch query tool."`
	Git        git.Options                 `cmd:"" name:"git" help:"Git-related utilities."`
	Gateway    gateway.Options             `cmd:"" name:"gateway" help:"Start a unified gateway server for all mu services."`
	Crypto     crypto.Options              `cmd:"" name:"crypto" help:"Crypto utilities."`
	Commit     commit.Options              `cmd:"" name:"commit" help:"Generate conventional commit message using AI."`
	Completion completion.Options          `cmd:"" name:"completion" help:"Generate shell completion script."`
}
