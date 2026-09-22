package downloader

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
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

	if o.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in via --insecure
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
