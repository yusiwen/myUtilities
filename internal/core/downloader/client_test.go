package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidateHeaders(t *testing.T) {
	cases := []struct {
		name    string
		headers []string
		user    string
		wantErr bool
	}{
		{name: "none"},
		{name: "valid", headers: []string{"X-Test: 1", "Accept: */*"}},
		{name: "missing colon", headers: []string{"X-Test"}, wantErr: true},
		{name: "empty key", headers: []string{": value"}, wantErr: true},
		{name: "user without colon", user: "alice", wantErr: true},
		{name: "user with colon", user: "alice:secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := Options{Headers: c.headers, User: c.user}
			if err := o.validateHeaders(); (err != nil) != c.wantErr {
				t.Errorf("validateHeaders() err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

func TestNewRequestAppliesHeadersAndAuth(t *testing.T) {
	o := Options{
		Headers: []string{"X-Test: 1", "X-Multi: a", "X-Multi: b"},
		Auth:    "token",
		User:    "alice:secret",
	}
	req, err := o.newRequest(context.Background(), http.MethodGet, "https://example.com/file.bin")
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if got := req.Header.Get("X-Test"); got != "1" {
		t.Errorf("X-Test = %q, want %q", got, "1")
	}
	if got := req.Header.Values("X-Multi"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("X-Multi = %v, want [a b]", got)
	}
	// --user takes precedence over --auth, matching the order the headers are set.
	if got := req.Header.Get("Authorization"); got != "Basic YWxpY2U6c2VjcmV0" {
		t.Errorf("Authorization = %q, want basic alice:secret", got)
	}
	if got := req.Header.Get("User-Agent"); !strings.HasPrefix(got, "mu/") {
		t.Errorf("User-Agent = %q, want a mu/<version> prefix", got)
	}

	bearer := Options{Auth: "token"}
	req, err = bearer.newRequest(context.Background(), http.MethodGet, "https://example.com/file.bin")
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer token" {
		t.Errorf("Authorization = %q, want bearer token", got)
	}
}

func TestNewClientTransportOptions(t *testing.T) {
	base := Options{Threads: 4, Timeout: 5 * time.Second}

	client, err := NewClient(base)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("caller did not receive a *http.Transport")
	}
	if !tr.ForceAttemptHTTP2 || tr.TLSNextProto != nil {
		t.Error("HTTP/2 should be enabled by default")
	}
	if tr.ResponseHeaderTimeout != base.Timeout {
		t.Errorf("ResponseHeaderTimeout = %s, want %s", tr.ResponseHeaderTimeout, base.Timeout)
	}

	http1 := base
	http1.HTTP1 = true
	client, err = NewClient(http1)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tr = client.Transport.(*http.Transport)
	if tr.ForceAttemptHTTP2 || tr.TLSNextProto == nil {
		t.Error("--http1 must disable HTTP/2 with an empty TLSNextProto map")
	}

	insecure := base
	insecure.Insecure = true
	client, err = NewClient(insecure)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tr = client.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("--insecure must skip certificate verification")
	}

	proxied := base
	proxied.Proxy = "http://127.0.0.1:3128"
	client, err = NewClient(proxied)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tr = client.Transport.(*http.Transport)
	target, err := url.Parse("http://example.com/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := tr.Proxy(&http.Request{URL: target})
	if err != nil {
		t.Fatalf("proxy lookup: %v", err)
	}
	if proxyURL == nil || proxyURL.Host != "127.0.0.1:3128" {
		t.Errorf("proxy = %v, want 127.0.0.1:3128", proxyURL)
	}

	broken := base
	broken.Proxy = "http://[::1]:namedport"
	if _, err := NewClient(broken); err == nil {
		t.Error("expected an error for an invalid proxy URL")
	}
}

func TestNewClientHTTP2Negotiation(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck // test server
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	opts := Options{Threads: 2, Timeout: 5 * time.Second, Insecure: true}
	client, err := NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Errorf("default protocol = %s, want HTTP/2", resp.Proto)
	}

	opts.HTTP1 = true
	client, err = NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err = client.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.ProtoMajor != 1 {
		t.Errorf("--http1 protocol = %s, want HTTP/1.1", resp.Proto)
	}
}

func TestNewClientInsecureTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck // test server
	}))
	defer srv.Close()

	strict, err := NewClient(Options{Threads: 1, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := strict.Get(srv.URL); err == nil {
		t.Error("expected a certificate verification failure without --insecure")
	}

	relaxed, err := NewClient(Options{Threads: 1, Timeout: 5 * time.Second, Insecure: true})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := relaxed.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET with --insecure: %v", err)
	}
	resp.Body.Close()
}
