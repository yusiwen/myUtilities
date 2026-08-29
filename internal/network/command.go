package network

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/likexian/whois"
	corehttp "github.com/yusiwen/myUtilities/internal/core/httpclient"
	corenet "github.com/yusiwen/myUtilities/internal/core/network"
)

// Options is the root `mu network` command.
//
// Two modes:
//   - Server mode:  `mu network serve` (or `mu network --server`)
//   - Client mode:  `mu network http <url>` (or `mu network --client <url>`)
//
// Subcommands dns/dig/whois/cert work the same as before.
type Options struct {
	Server bool `flag:"" name:"server" help:"Start the network-tools HTTP server on the default port."`

	// Subcommands.
	Serve  ServeOptions  `cmd:"" name:"serve" help:"Start the network-tools web server."`
	DNS    DNSOptions    `cmd:"" name:"dns" help:"DNS lookup."`
	DIG    DIGOptions    `cmd:"" name:"dig" help:"Detailed DNS query (dig-style)."`
	Whois  WhoisOptions  `cmd:"" name:"whois" help:"WHOIS lookup for domain or IP."`
	Cert   CertOptions   `cmd:"" name:"cert" help:"SSL/TLS certificate details."`
	HTTP   HTTPClientOpt `cmd:"" name:"http" help:"HTTP client (curl-like)."`
}

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

// Run handles the top-level `mu network` command. Kong routes subcommands
// (dns/dig/whois/cert/http/serve) themselves; this only handles the
// `--server` shortcut.
func (o *Options) Run() error {
	if o.Server {
		return (&ServeOptions{Port: 8091}).Run()
	}
	return fmt.Errorf("no subcommand specified. Try: mu network http|serve|dns|dig|whois|cert — run 'mu network -h' for help")
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
