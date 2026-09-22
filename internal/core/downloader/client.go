package downloader

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/yusiwen/myUtilities/internal/core/version"
)

// userAgent identifies the downloader to servers.
var userAgent = func() string {
	v := version.Version
	if v == "" {
		v = "dev"
	}
	return "mu/" + v + " (+https://github.com/yusiwen/myUtilities)"
}()

// NewClient builds the HTTP client used for probing and for the transfer.
//
// The client has no overall timeout: the caller controls connect/header waits
// through Transport.ResponseHeaderTimeout and body stalls through the idle
// watchdog in the worker.
func NewClient(o Options) (*http.Client, error) {
	dialer := &net.Dialer{
		Timeout:   o.Timeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          max(o.Threads*2, 16),
		MaxIdleConnsPerHost:   max(o.Threads*2, 8),
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   o.Timeout,
		ExpectContinueTimeout: time.Second,
	}

	if o.Timeout > 0 {
		transport.ResponseHeaderTimeout = o.Timeout
	}

	if o.HTTP1 {
		// An empty (non-nil) TLSNextProto map disables HTTP/2 negotiation, so
		// every worker gets its own TCP connection instead of a multiplexed
		// stream on a single one.
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}

	if o.Insecure || o.CACert != "" {
		tlsConfig, err := newTLSConfig(o)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}

	if o.Proxy != "" {
		proxyURL, err := url.Parse(o.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy %q: %w", o.Proxy, err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{Transport: transport}, nil
}

// newTLSConfig builds the TLS settings for --cacert and --insecure.
//
// --cacert appends the given PEM certificates to the platform/system roots, so
// private CAs (corporate TLS interception, internal services) work without
// disabling verification. On macOS the pool returned by SystemCertPool keeps its
// "system pool" marker, which lets the platform verifier run first and the Go
// verifier fall back to the appended roots; on Linux/BSD it is the bundle read
// from the standard CA paths (or SSL_CERT_FILE/SSL_CERT_DIR).
func newTLSConfig(o Options) (*tls.Config, error) {
	config := &tls.Config{}

	if o.CACert != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		pemData, err := os.ReadFile(o.CACert)
		if err != nil {
			return nil, fmt.Errorf("cannot read --cacert %s: %w", o.CACert, err)
		}
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("--cacert %s contains no usable certificates", o.CACert)
		}
		config.RootCAs = pool
	}

	if o.Insecure {
		// Opt-in via --insecure: skips chain and hostname verification.
		config.InsecureSkipVerify = true //nolint:gosec // explicit user request
	}
	return config, nil
}

// validateHeaders rejects malformed `Key: Value` header flags.
func (o Options) validateHeaders() error {
	for _, h := range o.Headers {
		key, _, found := strings.Cut(h, ":")
		if !found || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid header %q (expected \"Key: Value\")", h)
		}
	}
	if o.User != "" && !strings.Contains(o.User, ":") {
		return fmt.Errorf("invalid --user %q (expected \"user:password\")", o.User)
	}
	return nil
}

// newRequest builds a request carrying the configured headers and credentials.
func (o Options) newRequest(ctx context.Context, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for _, h := range o.Headers {
		key, value, _ := strings.Cut(h, ":")
		req.Header.Add(strings.TrimSpace(key), strings.TrimSpace(value))
	}
	if o.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+o.Auth)
	}
	if o.User != "" {
		user, pass, _ := strings.Cut(o.User, ":")
		req.SetBasicAuth(user, pass)
	}
	return req, nil
}
