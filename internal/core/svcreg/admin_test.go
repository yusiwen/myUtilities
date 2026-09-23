package svcreg

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// withDataRoot points the admin data root at a temporary directory for one test.
func withDataRoot(t *testing.T, root string) {
	t.Helper()
	old := stateDir
	stateDir = root
	t.Cleanup(func() { stateDir = old })
}

func adminMux() *http.ServeMux {
	mux := http.NewServeMux()
	RegisterAdminAPI(mux, &Client{Server: "http://127.0.0.1:30100"})
	return mux
}

// TestAdminAPIRejectsRemoteCallers verifies that the admin API is unreachable
// from anything but the local host, since it can start and stop a local server
// process and choose its database path.
func TestAdminAPIRejectsRemoteCallers(t *testing.T) {
	withDataRoot(t, t.TempDir())
	mux := adminMux()

	requests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/svcreg/admin/config"},
		{http.MethodGet, "/api/svcreg/admin/status"},
		{http.MethodPost, "/api/svcreg/admin/start"},
		{http.MethodPost, "/api/svcreg/admin/stop"},
	}
	for _, tc := range requests {
		for _, remote := range []string{"10.1.2.3:40000", "203.0.113.9:1234"} {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
			req.RemoteAddr = remote
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s from %s: got %d, want 403", tc.method, tc.path, remote, rec.Code)
			}
		}
	}
}

// TestAdminAPIAllowsLoopback verifies the dashboard on the same host still works.
func TestAdminAPIAllowsLoopback(t *testing.T) {
	withDataRoot(t, t.TempDir())
	mux := adminMux()

	req := httptest.NewRequest(http.MethodGet, "/api/svcreg/admin/config", nil)
	req.RemoteAddr = "127.0.0.1:55555"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback request: got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "defaultPort") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}

	// IPv6 loopback is local too.
	req = httptest.NewRequest(http.MethodGet, "/api/svcreg/admin/config", nil)
	req.RemoteAddr = "[::1]:55555"
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("IPv6 loopback request: got %d, want 200", rec.Code)
	}
}

// TestResolveDBPathRejectsEscapes covers the containment check on the
// client-supplied database path.
func TestResolveDBPathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	withDataRoot(t, root)

	ok := []string{
		"inside.db",
		filepath.Join(root, "sub", "inside.db"),
		filepath.Join(root, "..", filepath.Base(root), "sibling.db"),
	}
	for _, raw := range ok {
		got, err := resolveDBPath(raw)
		if err != nil {
			t.Errorf("resolveDBPath(%q) = error %v, want success", raw, err)
			continue
		}
		if !filepath.IsAbs(got) {
			t.Errorf("resolveDBPath(%q) = %q, want an absolute path", raw, got)
		}
		if !strings.HasPrefix(got, root) {
			t.Errorf("resolveDBPath(%q) = %q, want it inside %s", raw, got, root)
		}
	}

	bad := []string{
		"../escape.db",
		filepath.Join(root, "..", "..", "escape.db"),
		"/etc/passwd",
		"~/.config/mu/svcreg.db",
		filepath.Join(root+"-evil", "escape.db"),
	}
	for _, raw := range bad {
		if got, err := resolveDBPath(raw); err == nil {
			t.Errorf("resolveDBPath(%q) = %q, want an error", raw, got)
		}
	}
}

// TestValidateAdminConfig checks the request validation performed before the
// child process is spawned.
func TestValidateAdminConfig(t *testing.T) {
	root := t.TempDir()
	withDataRoot(t, root)

	valid := adminConfig{Port: 30101, Host: "127.0.0.1", DBPath: filepath.Join(root, "a.db")}
	if err := validateAdminConfig(&valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if !filepath.IsAbs(valid.DBPath) {
		t.Fatalf("DBPath not resolved to an absolute path: %q", valid.DBPath)
	}

	invalid := []adminConfig{
		{Port: 0, Host: "127.0.0.1", DBPath: filepath.Join(root, "a.db")},
		{Port: 70000, Host: "127.0.0.1", DBPath: filepath.Join(root, "a.db")},
		{Port: 30100, Host: "", DBPath: filepath.Join(root, "a.db")},
		{Port: 30100, Host: "-oops", DBPath: filepath.Join(root, "a.db")},
		{Port: 30100, Host: "127.0.0.1; rm -rf /", DBPath: filepath.Join(root, "a.db")},
		{Port: 30100, Host: "127.0.0.1", DBPath: "/tmp/outside.db"},
	}
	for _, cfg := range invalid {
		if err := validateAdminConfig(&cfg); err == nil {
			t.Errorf("config %+v: want an error, got nil", cfg)
		}
	}
}

// TestLogPathStaysInDataRoot verifies the serve log no longer follows the
// caller-supplied database path.
func TestLogPathStaysInDataRoot(t *testing.T) {
	root := t.TempDir()
	withDataRoot(t, root)

	m := &ServerManager{config: adminConfig{DBPath: "/somewhere/else/svcreg.db"}}
	got := m.logPath()
	if filepath.Dir(got) != root {
		t.Fatalf("logPath() = %q, want it directly under %s", got, root)
	}
}
