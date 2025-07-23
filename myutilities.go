package main

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/installer"
	"github.com/yusiwen/myUtilities/mock"
	"github.com/yusiwen/myUtilities/proxy"
)

type MyUtilities struct {
	Version   kong.VersionFlag  `short:"v" help:"Print the version number"`
	Installer installer.Options `cmd:"" name:"install" help:"Install binary from GitHub release."`
	Mocker    mock.Options      `cmd:"" name:"mock" help:"Mockers."`
	Proxy     proxy.Options     `cmd:"" name:"proxy" help:"Proxies."`
}
