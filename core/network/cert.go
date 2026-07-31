package network

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"strings"
	"time"
)

// CertInfo fetches and formats SSL/TLS certificate details for a domain.
func CertInfo(domain string, port int) (string, error) {
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
		fingerprint := sha256.Sum256(cert.Raw)
		b.WriteString(fmt.Sprintf("SHA-256:     %X\n", fingerprint))
	}
	return b.String(), nil
}
