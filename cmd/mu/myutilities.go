package main

import (
	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/ask"
	"github.com/yusiwen/myUtilities/internal/budget"
	"github.com/yusiwen/myUtilities/internal/completion"
	"github.com/yusiwen/myUtilities/internal/crypto"
	"github.com/yusiwen/myUtilities/internal/diff"
	"github.com/yusiwen/myUtilities/internal/es"
	"github.com/yusiwen/myUtilities/internal/fleet"
	"github.com/yusiwen/myUtilities/internal/gateway"
	"github.com/yusiwen/myUtilities/internal/git"
	"github.com/yusiwen/myUtilities/internal/installer"
	"github.com/yusiwen/myUtilities/internal/jarinfo"
	"github.com/yusiwen/myUtilities/internal/k8s"
	"github.com/yusiwen/myUtilities/internal/metrics"
	"github.com/yusiwen/myUtilities/internal/misc"
	"github.com/yusiwen/myUtilities/internal/mock"
	"github.com/yusiwen/myUtilities/internal/network"
	"github.com/yusiwen/myUtilities/internal/proxy"
	"github.com/yusiwen/myUtilities/internal/qrcode"
	"github.com/yusiwen/myUtilities/internal/runner"
	"github.com/yusiwen/myUtilities/internal/scip"
	"github.com/yusiwen/myUtilities/internal/serve"
	"github.com/yusiwen/myUtilities/internal/svcreg"
	"github.com/yusiwen/myUtilities/internal/watch"
	"github.com/yusiwen/myUtilities/internal/wol"
)

type MyUtilities struct {
	Version    kong.VersionFlag            `short:"v" help:"Print the version number"`
	Installer  installer.Options           `cmd:"" name:"install" help:"Install binary from GitHub release."`
	Mocker     mock.Options                `cmd:"" name:"mock" help:"Mockers."`
	Qrcode     qrcode.Options              `cmd:"" name:"qrcode" help:"Generate QR codes."`
	Serve      serve.Options               `cmd:"" name:"serve" help:"Start a static file server."`
	Svcreg     svcreg.Options              `cmd:"" name:"svcreg" help:"Service registry server (ServiceCenter-compatible)."`
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
	Network    network.Options             `cmd:"" name:"network" help:"Network tools (DNS, DIG)."`
	Misc       misc.Options                `cmd:"" name:"misc" help:"Miscellaneous tools (JSON, UUID, timestamp, hash)."`
	Crypto     crypto.Options              `cmd:"" name:"crypto" help:"Crypto utilities."`
	Ask        ask.Options                 `cmd:"" name:"ask" help:"Ask LLM questions."`
	Budget     budget.Options              `cmd:"" name:"budget" help:"Query LLM API usage and balance."`
	Metrics    metrics.Options             `cmd:"" name:"metrics" help:"Time-series metrics collection and querying."`
	Scip       scip.Options                `cmd:"" name:"scip" help:"SCIP semantic code intelligence."`
	Fleet      fleet.Options               `cmd:"" name:"fleet" help:"Fleet management (remote execution/deploys)."`
	Completion completion.Options          `cmd:"" name:"completion" help:"Generate shell completion script."`
}
