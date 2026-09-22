package network

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/likexian/whois"
	corehttp "github.com/yusiwen/myUtilities/internal/core/httpclient"
	corenet "github.com/yusiwen/myUtilities/internal/core/network"
)

// Options is the root `mu network` command.
//
// Subcommands cover DNS/DIG/WHOIS lookups, TLS certificate inspection, the
// curl-like HTTP client, the multi-threaded downloader and the port scanner.
// Two conveniences live on the parent: `mu network serve` (or the
// `mu network --server` shortcut) starts the network-tools web server, and a
// bare `mu network` prints the list of subcommands.
type Options struct {
	Server bool `flag:"" name:"server" help:"Start the network-tools HTTP server on the default port (same as 'mu network serve')."`

	// Bare is the hidden placeholder subcommand Kong selects when `mu network`
	// runs without a subcommand. Kong refuses to select a parent that has
	// subcommands, so this placeholder is what makes the bare invocation (and
	// the --server shortcut) reach Run below. Its name must stay in sync with
	// bareCmdName.
	Bare bareCommand `cmd:"" name:"bare" hidden:"" default:"1"`

	// Subcommands.
	Serve    ServeOptions    `cmd:"" name:"serve" help:"Start the network-tools web server."`
	DNS      DNSOptions      `cmd:"" name:"dns" help:"DNS lookup."`
	DIG      DIGOptions      `cmd:"" name:"dig" help:"Detailed DNS query (dig-style)."`
	Whois    WhoisOptions    `cmd:"" name:"whois" help:"WHOIS lookup for domain or IP."`
	Cert     CertOptions     `cmd:"" name:"cert" help:"SSL/TLS certificate details."`
	HTTP     HTTPClientOpt   `cmd:"" name:"http" help:"HTTP client (curl-like)."`
	Download DownloadOptions `cmd:"" name:"download" help:"Multi-threaded resumable file download (HTTP/HTTPS)."`
	PortScan PortScanOptions `cmd:"" name:"port-scan" help:"Port scan: local listeners or remote TCP probe."`
}

// bareCmdName is the name of the hidden default subcommand declared above.
const bareCmdName = "bare"

// bareCommand is the hidden default subcommand of `mu network`. It does
// nothing itself; Options.Run handles the bare invocation.
type bareCommand struct{}

// Run is a no-op: Options.Run decides what a bare `mu network` invocation does.
func (b *bareCommand) Run() error { return nil }

// HTTPClientOpt is the CLI surface for `mu network http`.
// It mirrors the flags previously on `mu http` so existing muscle memory and
// docs stay valid: `mu network http -X POST -d @body.json https://x/api`.
type HTTPClientOpt struct {
	URL      string   `arg:"" name:"url" help:"Target URL." required:""`
	Method   string   `short:"X" name:"method" help:"HTTP method." default:"GET" enum:"GET,POST,PUT,PATCH,DELETE,HEAD,OPTIONS"`
	Headers  []string `short:"H" name:"header" help:"Request headers as Key: Value (repeatable)."`
	Data     string   `short:"d" name:"data" help:"Request body (or pipe from stdin)."`
	Auth     string   `short:"A" name:"auth" help:"Bearer token for Authorization header."`
	Timeout  string   `short:"t" name:"timeout" help:"Request timeout (e.g. 30s, 2m)." default:"30s"`
	Insecure bool     `short:"k" name:"insecure" help:"Skip TLS certificate verification."`
	NoFollow bool     `short:"N" name:"no-follow" help:"Do not follow redirects."`
	JSON     bool     `short:"j" name:"json" help:"Force pretty-print JSON response."`
	BodyOnly bool     `short:"b" name:"body" help:"Print only the response body (no headers/status)."`
	Output   string   `short:"o" name:"output" help:"Write response body to file instead of stdout."`
}

// Run handles the top-level `mu network` command: the `--server` shortcut and
// the bare invocation.
//
// Kong invokes the Run method of every node on the selected command path (leaf
// first, then its parents), so when a real subcommand was selected this must
// stay silent — otherwise every `mu network <sub>` would exit non-zero. The
// hidden `bare` subcommand is the only other node that reaches this method.
func (o *Options) Run(ctx *kong.Context) error {
	if selected := ctx.Selected(); selected != nil && selected.Name != bareCmdName {
		return nil
	}
	if o.Server {
		return (&ServeOptions{Port: 8091}).Run()
	}
	return fmt.Errorf("no subcommand specified. Try: mu network http|download|serve|dns|dig|whois|cert|port-scan — run 'mu network -h' for help")
}

func (o *HTTPClientOpt) Run() error {
	timeout, err := time.ParseDuration(o.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", o.Timeout, err)
	}

	body := o.Data
	if body == "" {
		if body, err = corehttp.ReadBodyFromStdin(); err != nil {
			return err
		}
	}

	p := corehttp.Params{
		URL:      o.URL,
		Method:   o.Method,
		Headers:  o.Headers,
		Body:     body,
		Auth:     o.Auth,
		Timeout:  timeout,
		Insecure: o.Insecure,
		NoFollow: o.NoFollow,
		JSON:     o.JSON,
		BodyOnly: o.BodyOnly,
		Output:   o.Output,
	}

	res, err := corehttp.Do(p)
	if err != nil {
		return err
	}

	fmt.Print(corehttp.Render(p, res))
	fmt.Fprintln(os.Stderr, corehttp.SummaryLine(p, res))
	return nil
}

type DNSOptions struct {
	Host string `arg:"" name:"host" help:"Hostname to look up."`
	Type string `short:"t" name:"type" enum:"A,AAAA,MX,NS,CNAME,TXT,SOA,ALL" default:"A" help:"DNS record type."`
}

type DIGOptions struct {
	Host string `arg:"" name:"host" help:"Hostname to query."`
	Type string `short:"t" name:"type" enum:"A,AAAA,MX,NS,CNAME,TXT,SOA" default:"A" help:"DNS record type."`
	Ns   string `short:"n" name:"ns" help:"Nameserver to query (e.g. 8.8.8.8)."`
}

type WhoisOptions struct {
	Domain string `arg:"" name:"domain" help:"Domain name or IP to look up."`
}

type CertOptions struct {
	Domain string `arg:"" name:"domain" help:"Domain name to check."`
	Port   int    `short:"p" name:"port" help:"Port to connect to." default:"443"`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8091"`
}

func (o *DNSOptions) Run() error {
	results, queryTime, err := corenet.LookupDNS(o.Host, o.Type)
	if err != nil {
		return err
	}
	for _, r := range results {
		fmt.Printf("%-5s %s (TTL: %d)\n", r.Type, r.Value, r.TTL)
	}
	fmt.Fprintf(os.Stderr, "Query time: %d ms\n", queryTime)
	return nil
}

func (o *DIGOptions) Run() error {
	out, err := corenet.Dig(o.Host, o.Type, o.Ns)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func (o *WhoisOptions) Run() error {
	result, err := whois.Whois(o.Domain)
	if err != nil {
		return fmt.Errorf("whois lookup failed: %w", err)
	}
	fmt.Print(result)
	return nil
}

func (o *CertOptions) Run() error {
	result, err := corenet.CertInfo(o.Domain, o.Port)
	if err != nil {
		return err
	}
	fmt.Print(result)
	return nil
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Network tools server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

// RegisterHandlers registers the network API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux) {
	corenet.RegisterHandlers(mux)
}
