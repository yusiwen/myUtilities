package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}

func newTestServer(t *testing.T, path string) *httptest.Server {
	t.Helper()
	h, err := NewFileHandler(path)
	if err != nil {
		t.Fatalf("NewFileHandler: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestFileHandlerServesAtRootAndName(t *testing.T) {
	path := newTestFile(t, "data.zip", "payload")
	srv := newTestServer(t, path)

	for _, target := range []string{"/", "/data.zip"} {
		resp, err := http.Get(srv.URL + target)
		if err != nil {
			t.Fatalf("GET %s: %v", target, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", target, resp.StatusCode)
		}
		if string(body) != "payload" {
			t.Errorf("GET %s body = %q, want payload", target, body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
			t.Errorf("GET %s Content-Type = %q, want application/zip", target, ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "data.zip") {
			t.Errorf("GET %s Content-Disposition = %q, want an attachment named data.zip", target, cd)
		}
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s X-Content-Type-Options = %q, want nosniff", target, got)
		}
	}
}

func TestFileHandlerSupportsRange(t *testing.T) {
	path := newTestFile(t, "big.bin", "0123456789")
	srv := newTestServer(t, path)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with Range: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if string(body) != "2345" {
		t.Errorf("body = %q, want 2345", body)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Errorf("Content-Range = %q, want bytes 2-5/10", got)
	}
}

func TestFileHandlerHead(t *testing.T) {
	srv := newTestServer(t, newTestFile(t, "data.zip", "payload"))

	resp, err := http.Head(srv.URL + "/")
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if len(body) != 0 {
		t.Errorf("HEAD returned %d body bytes, want none", len(body))
	}
	if got := resp.Header.Get("Content-Length"); got != "7" {
		t.Errorf("Content-Length = %q, want 7", got)
	}
}

func TestFileHandlerRejectsOtherPathsAndMethods(t *testing.T) {
	srv := newTestServer(t, newTestFile(t, "data.zip", "payload"))

	resp, err := http.Get(srv.URL + "/etc/passwd")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status for an unknown path = %d, want 404", resp.StatusCode)
	}

	post, err := http.Post(srv.URL+"/", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status for POST = %d, want 405", post.StatusCode)
	}
	if allow := post.Header.Get("Allow"); allow != "GET, HEAD" {
		t.Errorf("Allow = %q, want GET, HEAD", allow)
	}
}

func TestFileHandlerEncodesNonASCIINames(t *testing.T) {
	path := newTestFile(t, "测试 包.zip", "payload")
	srv := newTestServer(t, path)

	resp, err := http.Get(srv.URL + "/" + url.PathEscape("测试 包.zip"))
	if err != nil {
		t.Fatalf("GET non-ASCII name: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "filename*=utf-8''") {
		t.Errorf("Content-Disposition = %q, want an RFC 2231 filename* parameter", cd)
	}
}

func TestNewFileHandlerRejectsDirectories(t *testing.T) {
	if _, err := NewFileHandler(t.TempDir()); err == nil {
		t.Fatal("NewFileHandler accepted a directory")
	}
	if _, err := NewFileHandler(filepath.Join(t.TempDir(), "missing.zip")); err == nil {
		t.Fatal("NewFileHandler accepted a missing file")
	}
}

func TestContentTypeFor(t *testing.T) {
	cases := map[string]string{
		"a.zip":        "application/zip",
		"a.tar":        "application/x-tar",
		"a.bin":        "application/octet-stream",
		"a":            "application/octet-stream",
		"a.html":       "text/html; charset=utf-8",
		"a.unknownext": "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestNewDownloadServerLeavesTransfersUnbounded pins the timeout fix: a download
// server must not cap the response write (that truncates large files), while the
// slowloris defences stay in place.
func TestNewDownloadServerLeavesTransfersUnbounded(t *testing.T) {
	srv := NewDownloadServer("127.0.0.1:0", http.NewServeMux())
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (no cap on the transfer)", srv.WriteTimeout)
	}
	if srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 (no cap on the transfer)", srv.ReadTimeout)
	}
	if srv.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout must stay set to defend against slow headers")
	}
	if srv.IdleTimeout == 0 {
		t.Error("IdleTimeout must stay set so idle keep-alive connections are reaped")
	}
}

func TestFileHandlerToken(t *testing.T) {
	h, err := NewFileHandler(newTestFile(t, "data.zip", "payload"))
	if err != nil {
		t.Fatalf("NewFileHandler: %v", err)
	}
	h.Token = "s3cret"
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cases := []struct {
		target string
		want   int
	}{
		{"/", http.StatusNotFound},
		{"/data.zip", http.StatusNotFound},
		{"/wrong", http.StatusNotFound},
		{"/s3cretx", http.StatusNotFound},
		{"/s3cret", http.StatusOK},
		{"/s3cret/", http.StatusOK},
		{"/s3cret/data.zip", http.StatusOK},
	}
	for _, c := range cases {
		resp, err := http.Get(srv.URL + c.target)
		if err != nil {
			t.Fatalf("GET %s: %v", c.target, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("GET %s status = %d, want %d", c.target, resp.StatusCode, c.want)
		}
		if c.want == http.StatusOK && string(body) != "payload" {
			t.Errorf("GET %s body = %q, want payload", c.target, body)
		}
	}

	// HEAD goes through the same token check.
	resp, err := http.Head(srv.URL + "/s3cret")
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD /s3cret status = %d, want 200", resp.StatusCode)
	}
}

func TestFileHandlerExpiry(t *testing.T) {
	h, err := NewFileHandler(newTestFile(t, "data.zip", "payload"))
	if err != nil {
		t.Fatalf("NewFileHandler: %v", err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	// A link that has not expired yet is served normally.
	h.ExpiresAt = time.Now().Add(time.Hour)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status before expiry = %d, want 200", resp.StatusCode)
	}

	h.ExpiresAt = time.Now().Add(-time.Second)
	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Errorf("status after expiry = %d, want 410", resp.StatusCode)
	}
	if !strings.Contains(string(body), "expired") {
		t.Errorf("body = %q, want a note that the link expired", body)
	}
}

func TestFileHandlerOnCompleteFiresOnlyForWholeFile(t *testing.T) {
	path := newTestFile(t, "data.bin", "0123456789")

	tests := []struct {
		name   string
		method string
		header map[string]string
		want   int
	}{
		{name: "whole file", method: http.MethodGet, want: 1},
		{name: "head", method: http.MethodHead, want: 0},
		{name: "partial range", method: http.MethodGet, header: map[string]string{"Range": "bytes=0-3"}, want: 0},
		{name: "full range", method: http.MethodGet, header: map[string]string{"Range": "bytes=0-"}, want: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewFileHandler(path)
			if err != nil {
				t.Fatalf("NewFileHandler: %v", err)
			}
			var calls int
			h.OnComplete = func() { calls++ }

			req := httptest.NewRequest(tc.method, "/", nil)
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if calls != tc.want {
				t.Errorf("OnComplete called %d times, want %d", calls, tc.want)
			}
		})
	}
}

// truncatedWriter simulates a client that disconnects mid-download.
type truncatedWriter struct {
	header  http.Header
	limit   int
	written int
}

func (w *truncatedWriter) Header() http.Header { return w.header }
func (w *truncatedWriter) WriteHeader(int)     {}
func (w *truncatedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, io.ErrClosedPipe
	}
	if len(p) > remaining {
		w.written += remaining
		return remaining, io.ErrClosedPipe
	}
	w.written += len(p)
	return len(p), nil
}

func TestFileHandlerAbortedDownloadDoesNotComplete(t *testing.T) {
	h, err := NewFileHandler(newTestFile(t, "data.bin", "0123456789"))
	if err != nil {
		t.Fatalf("NewFileHandler: %v", err)
	}
	var calls int
	h.OnComplete = func() { calls++ }

	w := &truncatedWriter{header: make(http.Header), limit: 4}
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if calls != 0 {
		t.Errorf("OnComplete fired after a truncated transfer, want no call")
	}
}
