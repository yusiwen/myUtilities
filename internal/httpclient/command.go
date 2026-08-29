// Package httpclient is a thin CLI wrapper that exposes the curl-like HTTP
// client from internal/core/httpclient as the `mu network http` subcommand.
//
// The standalone `mu http` alias is kept by registering this package's
// Options under `cmd:"" name:"http"` in cmd/mu/myutilities.go. Both entry
// points ultimately call into corehttp.Do / corehttp.Render.
package httpclient

import (
	"fmt"
	"os"
	"time"

	corehttp "github.com/yusiwen/myUtilities/internal/core/httpclient"
)

// Options matches the historical `mu http` flag set. Kept as a top-level
// command so existing muscle memory and muscle-memory docs keep working.
type Options struct {
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

// Run executes the request and writes the response to stdout/stderr.
func (o *Options) Run() error {
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
