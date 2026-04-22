package main

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/installer"
	"github.com/yusiwen/myUtilities/mock"
	"github.com/yusiwen/myUtilities/proxy"
	"github.com/yusiwen/myUtilities/runner"
	"github.com/yusiwen/myUtilities/wol"
)

type MyUtilities struct {
	Version   kong.VersionFlag            `short:"v" help:"Print the version number"`
	Installer installer.Options           `cmd:"" name:"install" help:"Install binary from GitHub release."`
	Mocker    mock.Options                `cmd:"" name:"mock" help:"Mockers."`
	Proxy     proxy.Options               `cmd:"" name:"proxy" help:"Proxies."`
	Runner    runner.CommandRunnerOptions `cmd:"" name:"run" help:"Run commands."`
	Wol       wol.Options                 `cmd:"" name:"wol" help:"Wake-on-Lan HTTP server."`
}
