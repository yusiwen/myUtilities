package network

import (
	"fmt"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type DNSResult struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	TTL   uint32 `json:"ttl"`
	Value string `json:"value"`
}

// LookupDNS resolves a host by record type using the system DNS (8.8.8.8).
func LookupDNS(host, recordType string) ([]DNSResult, int64, error) {
	start := time.Now()
	var allResults []DNSResult

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

func lookupByType(host, rtype string) ([]DNSResult, error) {
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

	var results []DNSResult
	for _, ans := range r.Answer {
		results = append(results, answerToResult(host, ans)...)
	}
	return results, nil
}

func answerToResult(host string, ans dns.RR) []DNSResult {
	h := ans.Header()
	switch v := ans.(type) {
	case *dns.A:
		return []DNSResult{{Name: host, Type: "A", TTL: h.Ttl, Value: v.A.String()}}
	case *dns.AAAA:
		return []DNSResult{{Name: host, Type: "AAAA", TTL: h.Ttl, Value: v.AAAA.String()}}
	case *dns.MX:
		return []DNSResult{{Name: host, Type: "MX", TTL: h.Ttl, Value: fmt.Sprintf("%s %d", v.Mx, v.Preference)}}
	case *dns.NS:
		return []DNSResult{{Name: host, Type: "NS", TTL: h.Ttl, Value: v.Ns}}
	case *dns.CNAME:
		return []DNSResult{{Name: host, Type: "CNAME", TTL: h.Ttl, Value: v.Target}}
	case *dns.TXT:
		return []DNSResult{{Name: host, Type: "TXT", TTL: h.Ttl, Value: strings.Join(v.Txt, " ")}}
	case *dns.SOA:
		return []DNSResult{{Name: host, Type: "SOA", TTL: h.Ttl, Value: fmt.Sprintf("%s %s (serial=%d)", v.Ns, v.Mbox, v.Serial)}}
	}
	return nil
}

// Dig runs a dig-style DNS query with full section output.
func Dig(host, rtype, ns string) (string, error) {
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
	if r.MsgHdr.Response {
		flags += "qr "
	}
	if r.MsgHdr.Authoritative {
		flags += "aa "
	}
	if r.MsgHdr.Truncated {
		flags += "tc "
	}
	if r.MsgHdr.RecursionDesired {
		flags += "rd "
	}
	if r.MsgHdr.RecursionAvailable {
		flags += "ra "
	}
	if r.MsgHdr.AuthenticatedData {
		flags += "ad "
	}
	if r.MsgHdr.CheckingDisabled {
		flags += "cd "
	}
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
