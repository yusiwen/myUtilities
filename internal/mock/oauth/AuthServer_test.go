package oauth

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// newTestAuthServer starts the mock OAuth server on a loopback listener.
func newTestAuthServer(t *testing.T) (*httptest.Server, *AuthServer) {
	t.Helper()
	s := NewAuthServer()
	mux := http.NewServeMux()
	s.SetupRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, s
}

// runParallel runs fn n times concurrently and reports every error.
func runParallel(t *testing.T, n int, fn func(i int) error) {
	t.Helper()
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := fn(i); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestConcurrentClientRegistration fires parallel writes at /clients. Before
// the handler lock this aborted the whole process with
// "fatal error: concurrent map writes".
func TestConcurrentClientRegistration(t *testing.T) {
	srv, s := newTestAuthServer(t)

	s.mu.Lock()
	before := len(s.clients)
	s.mu.Unlock()

	const n = 100
	runParallel(t, n, func(i int) error {
		body := fmt.Sprintf(
			`{"clientId":"c%d","clientName":"n%d","clientSecret":"s%d","redirectUri":"http://localhost/cb"}`,
			i, i, i)
		resp, err := http.Post(srv.URL+"/clients", "application/json", strings.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("client %d: status %d", i, resp.StatusCode)
		}
		return nil
	})

	s.mu.Lock()
	after := len(s.clients)
	s.mu.Unlock()
	if after != before+n {
		t.Fatalf("registered %d clients, want %d", after-before, n)
	}
}

// TestConcurrentLogin fires parallel writes at /login (the sessions map).
func TestConcurrentLogin(t *testing.T) {
	srv, _ := newTestAuthServer(t)

	// Do not follow the post-login redirect: the 302 itself is the assertion.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	const n = 100
	runParallel(t, n, func(i int) error {
		resp, err := client.PostForm(srv.URL+"/login", url.Values{
			"username": {"alice"},
			"password": {"password123"},
		})
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			return fmt.Errorf("login %d: status %d", i, resp.StatusCode)
		}
		return nil
	})
}

// TestConcurrentMixedHandlers mixes reads and writes across the whole handler
// set, including the endpoints that only read (or delete) state.
func TestConcurrentMixedHandlers(t *testing.T) {
	srv, _ := newTestAuthServer(t)

	type request struct {
		method      string
		path        string
		body        string
		contentType string
	}
	cases := func(i int) []request {
		return []request{
			{method: http.MethodGet, path: "/"},
			{method: http.MethodGet, path: "/clients"},
			{
				method:      http.MethodPost,
				path:        "/clients",
				body:        fmt.Sprintf(`{"clientId":"m%d","clientName":"x","clientSecret":"y","redirectUri":"http://localhost/cb"}`, i),
				contentType: "application/json",
			},
			{
				method:      http.MethodPost,
				path:        "/token",
				body:        "grant_type=authorization_code&code=nope&client_id=client1&client_secret=secret1",
				contentType: "application/x-www-form-urlencoded",
			},
			{method: http.MethodGet, path: "/userinfo?access_token=nope"},
			{method: http.MethodGet, path: "/verify?token=nope"},
			{method: http.MethodGet, path: "/login?request_id=x&client_id=client1"},
		}
	}

	const n = 40
	runParallel(t, n, func(i int) error {
		for _, c := range cases(i) {
			var body io.Reader
			if c.body != "" {
				body = strings.NewReader(c.body)
			}
			req, err := http.NewRequest(c.method, srv.URL+c.path, body)
			if err != nil {
				return err
			}
			if c.contentType != "" {
				req.Header.Set("Content-Type", c.contentType)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("%s %s: %w", c.method, c.path, err)
			}
			resp.Body.Close()
		}
		return nil
	})
}
