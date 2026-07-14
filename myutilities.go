package main

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/completion"
	"github.com/yusiwen/myUtilities/crypto"
	"github.com/yusiwen/myUtilities/diff"
	"github.com/yusiwen/myUtilities/es"
	"github.com/yusiwen/myUtilities/gateway"
	"github.com/yusiwen/myUtilities/git"
	"github.com/yusiwen/myUtilities/installer"
	"github.com/yusiwen/myUtilities/jarinfo"
	"github.com/yusiwen/myUtilities/k8s"
	"github.com/yusiwen/myUtilities/mock"
	"github.com/yusiwen/myUtilities/proxy"
	"github.com/yusiwen/myUtilities/qrcode"
	"github.com/yusiwen/myUtilities/runner"
	"github.com/yusiwen/myUtilities/serve"
	"github.com/yusiwen/myUtilities/watch"
	"github.com/yusiwen/myUtilities/wol"
)

type MyUtilities struct {
	Version    kong.VersionFlag            `short:"v" help:"Print the version number"`
	Installer  installer.Options           `cmd:"" name:"install" help:"Install binary from GitHub release."`
	Mocker     mock.Options                `cmd:"" name:"mock" help:"Mockers."`
	Qrcode     qrcode.Options              `cmd:"" name:"qrcode" help:"Generate QR codes."`
	Serve      serve.Options               `cmd:"" name:"serve" help:"Start a static file server."`
	Proxy      proxy.Options               `cmd:"" name:"proxy" help:"Proxies."`
	Runner     runner.CommandRunnerOptions `cmd:"" name:"run" help:"Run commands."`
	Wol        wol.Options                 `cmd:"" name:"wol" help:"Wake-on-Lan HTTP server."`
	Es         es.Options                  `cmd:"" name:"es" help:"Elasticsearch query tool."`
	Git        git.Options                 `cmd:"" name:"git" help:"Git-related utilities."`
	Watch      watch.Options               `cmd:"" name:"watch" help:"Watch resources for changes."`
	K8s        k8s.Options                 `cmd:"" name:"k8s" help:"Kubernetes utilities."`
	Jar        jarinfo.Options             `cmd:"" name:"jar" help:"Jar utilities."`
	Gateway    gateway.Options             `cmd:"" name:"gateway" help:"Start a unified gateway server for all mu services."`
	Diff       diff.Options                `cmd:"" name:"diff" help:"Text diff tool."`
	Crypto     crypto.Options              `cmd:"" name:"crypto" help:"Crypto utilities."`
	Completion completion.Options          `cmd:"" name:"completion" help:"Generate shell completion script."`
}
