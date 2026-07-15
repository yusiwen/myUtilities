package network

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/likexian/whois"
	"github.com/miekg/dns"
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
	results, queryTime, err := dnsLookup(o.Host, o.Type)
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
	out, err := digQuery(o.Host, o.Type, o.Ns)
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
	result, err := certInfo(o.Domain, o.Port)
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

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/network/dns", handleDNS)
	mux.HandleFunc("/api/network/dig", handleDIG)
	mux.HandleFunc("/api/network/whois", handleWhois)
	mux.HandleFunc("/api/network/cert", handleCert)
}

// DNS Lookup — standard library approach

type dnsResult struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	TTL   uint32 `json:"ttl"`
	Value string `json:"value"`
}

func dnsLookup(host, recordType string) ([]dnsResult, int64, error) {
	start := time.Now()
	var allResults []dnsResult

	types := []string{recordType}
	if recordType == "ALL" {
		types = []string{"A", "AAAA", "MX", "NS", "CNAME", "TXT", "SOA"}
	}

	for _, t := range types {
		results, err := lookupByType(host, t)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}
	queryTime := time.Since(start).Milliseconds()
	if len(allResults) == 0 {
		return nil, queryTime, fmt.Errorf("no records found for %s (type: %s)", host, recordType)
	}
	return allResults, queryTime, nil
}

func lookupByType(host, rtype string) ([]dnsResult, error) {
	c := dns.Client{SingleInflight: true}
	m := dns.Msg{}
	m.SetQuestion(dns.Fqdn(host), dns.StringToType[rtype])

	r, _, err := c.Exchange(&m, "8.8.8.8:53")
	if err != nil {
		return nil, err
	}
	if r.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS response code: %s", dns.RcodeToString[r.Rcode])
	}

	var results []dnsResult
	for _, ans := range r.Answer {
		results = append(results, answerToResult(host, ans)...)
	}
	return results, nil
}

func answerToResult(host string, ans dns.RR) []dnsResult {
	h := ans.Header()
	switch v := ans.(type) {
	case *dns.A:
		return []dnsResult{{Name: host, Type: "A", TTL: h.Ttl, Value: v.A.String()}}
	case *dns.AAAA:
		return []dnsResult{{Name: host, Type: "AAAA", TTL: h.Ttl, Value: v.AAAA.String()}}
	case *dns.MX:
		return []dnsResult{{Name: host, Type: "MX", TTL: h.Ttl, Value: fmt.Sprintf("%s %d", v.Mx, v.Preference)}}
	case *dns.NS:
		return []dnsResult{{Name: host, Type: "NS", TTL: h.Ttl, Value: v.Ns}}
	case *dns.CNAME:
		return []dnsResult{{Name: host, Type: "CNAME", TTL: h.Ttl, Value: v.Target}}
	case *dns.TXT:
		return []dnsResult{{Name: host, Type: "TXT", TTL: h.Ttl, Value: strings.Join(v.Txt, " ")}}
	case *dns.SOA:
		return []dnsResult{{Name: host, Type: "SOA", TTL: h.Ttl, Value: fmt.Sprintf("%s %s (serial=%d)", v.Ns, v.Mbox, v.Serial)}}
	}
	return nil
}

// DIG — miekg/dns full output

func digQuery(host, rtype, ns string) (string, error) {
	if ns == "" {
		ns = "8.8.8.8:53"
	} else if !strings.Contains(ns, ":") {
		ns = ns + ":53"
	}

	c := dns.Client{SingleInflight: true}
	m := dns.Msg{}
	m.SetQuestion(dns.Fqdn(host), dns.StringToType[rtype])

	start := time.Now()
	r, _, err := c.Exchange(&m, ns)
	queryTime := time.Since(start).Milliseconds()
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("; <<>> mu network dig <<>> %s\n", host))
	b.WriteString(fmt.Sprintf(";; status: %s, id: %d\n", dns.RcodeToString[r.Rcode], r.MsgHdr.Id))
	flags := ""
	if r.MsgHdr.Response { flags += "qr " }
	if r.MsgHdr.Authoritative { flags += "aa " }
	if r.MsgHdr.Truncated { flags += "tc " }
	if r.MsgHdr.RecursionDesired { flags += "rd " }
	if r.MsgHdr.RecursionAvailable { flags += "ra " }
	if r.MsgHdr.AuthenticatedData { flags += "ad " }
	if r.MsgHdr.CheckingDisabled { flags += "cd " }
	b.WriteString(fmt.Sprintf(";; flags: %s; QUERY: %d, ANSWER: %d, AUTHORITY: %d, ADDITIONAL: %d\n",
		flags, len(r.Question), len(r.Answer), len(r.Ns), len(r.Extra)))

	if len(r.Question) > 0 {
		b.WriteString("\n;; QUESTION SECTION:\n")
		for _, q := range r.Question {
			b.WriteString(fmt.Sprintf(";%s.\t\tIN\t%s\n", q.Name, dns.TypeToString[q.Qtype]))
		}
	}
	if len(r.Answer) > 0 {
		b.WriteString("\n;; ANSWER SECTION:\n")
		for _, ans := range r.Answer {
			b.WriteString(fmt.Sprintf("%s\n", ans.String()))
		}
	}
	if len(r.Ns) > 0 {
		b.WriteString("\n;; AUTHORITY SECTION:\n")
		for _, ns := range r.Ns {
			b.WriteString(fmt.Sprintf("%s\n", ns.String()))
		}
	}
	if len(r.Extra) > 0 {
		b.WriteString("\n;; ADDITIONAL SECTION:\n")
		for _, extra := range r.Extra {
			b.WriteString(fmt.Sprintf("%s\n", extra.String()))
		}
	}
	b.WriteString(fmt.Sprintf("\n;; Query time: %d msec\n", queryTime))
	b.WriteString(fmt.Sprintf(";; SERVER: %s\n", ns))
	b.WriteString(fmt.Sprintf(";; MSG SIZE: %d bytes\n", r.Len()))
	return b.String(), nil
}

// API handlers

func certInfo(domain string, port int) (string, error) {
	addr := fmt.Sprintf("%s:%d", domain, port)
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "", fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()

	var b strings.Builder
	for _, cert := range conn.ConnectionState().PeerCertificates {
		b.WriteString(fmt.Sprintf("Subject:     %s\n", cert.Subject))
		b.WriteString(fmt.Sprintf("Issuer:      %s\n", cert.Issuer))
		b.WriteString(fmt.Sprintf("Serial:      %X\n", cert.SerialNumber))
		b.WriteString(fmt.Sprintf("Version:     %d\n", cert.Version))
		b.WriteString(fmt.Sprintf("Not Before:  %s\n", cert.NotBefore.Format(time.RFC3339)))
		b.WriteString(fmt.Sprintf("Not After:   %s\n", cert.NotAfter.Format(time.RFC3339)))
		b.WriteString(fmt.Sprintf("DNS SANs:    %s\n", strings.Join(cert.DNSNames, ", ")))
		if len(cert.EmailAddresses) > 0 {
			b.WriteString(fmt.Sprintf("Email:       %s\n", strings.Join(cert.EmailAddresses, ", ")))
		}
		// SHA-256 fingerprint
		fingerprint := sha256.Sum256(cert.Raw)
		b.WriteString(fmt.Sprintf("SHA-256:     %X\n", fingerprint))
	}
	return b.String(), nil
}

func handleDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Host string `json:"host"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "A"
	}
	results, queryTime, err := dnsLookup(req.Host, req.Type)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results":   results,
		"queryTime": queryTime,
	})
}

func handleDIG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Host string `json:"host"`
		Type string `json:"type"`
		Ns   string `json:"ns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "A"
	}
	out, err := digQuery(req.Host, req.Type, req.Ns)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"dig": out})
}

func handleWhois(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	result, err := whois.Whois(req.Domain)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"whois": result})
}

func handleCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Domain string `json:"domain"`
		Port   int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Port == 0 {
		req.Port = 443
	}
	result, err := certInfo(req.Domain, req.Port)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"cert": result})
}
