package network

import (
	"fmt"
	"net/http"
	"os"

	"github.com/likexian/whois"
	corenet "github.com/yusiwen/myUtilities/internal/core/network"
)

type Options struct {
	DNS   DNSOptions   `cmd:"" name:"dns" help:"DNS lookup."`
	DIG   DIGOptions   `cmd:"" name:"dig" help:"Detailed DNS query (dig-style)."`
	Whois WhoisOptions `cmd:"" name:"whois" help:"WHOIS lookup for domain or IP."`
	Cert  CertOptions  `cmd:"" name:"cert" help:"SSL/TLS certificate details."`
	Serve ServeOptions `cmd:"" name:"serve" help:"Start network tools HTTP server."`
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
