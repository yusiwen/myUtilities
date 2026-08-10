package installer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestGetSendsToken verifies the token configured on the Client is actually
// attached to outgoing requests (regression: the request used to be built and
// then discarded in favor of http.Get).
func TestGetSendsToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v1.0.0"}`))
	}))
	defer srv.Close()

	c := &Client{Token: "ghp_123"}
	var out ghRelease
	if err := c.get(srv.URL, &out); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if gotAuth != "token ghp_123" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "token ghp_123")
	}
	if out.TagName != "v1.0.0" {
		t.Fatalf("unexpected tag: %q", out.TagName)
	}
}

func TestGetNoToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if err := (&Client{}).get(srv.URL, &map[string]interface{}{}); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header, got %q", gotAuth)
	}
}

// TestGetAssetsPagination verifies the release-tag branch pages through the
// releases list instead of only looking at the first page.
func TestGetAssetsPagination(t *testing.T) {
	perPage = 2
	defer func() { perPage = 100 }()
	oldAPI := apiBase
	apiBase = ""
	defer func() { apiBase = oldAPI }()

	var pages []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/tool/releases":
			pages = append(pages, r.URL.RawQuery)
			release := func(tag string) ghRelease {
				return ghRelease{TagName: tag, AssetsURL: srv.URL + "/assets/" + tag}
			}
			var out []ghRelease
			if r.URL.Query().Get("page") == "1" {
				out = []ghRelease{release("v1.0.0"), release("v1.1.0")}
			} else if r.URL.Query().Get("page") == "2" {
				out = []ghRelease{release("v2.0.0")}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)
		case "/assets/v1.1.0":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]ghAsset{
				{Name: "tool_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/tool_linux_amd64.tar.gz", Size: 5 * 1024 * 1024},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	apiBase = srv.URL

	c := &Client{}
	release, assets, err := c.getAssets(Query{User: "acme", Program: "tool", Release: "v1.1.0"})
	if err != nil {
		t.Fatalf("getAssets failed: %v", err)
	}
	if release != "v1.1.0" {
		t.Fatalf("release = %q, want v1.1.0", release)
	}
	if len(assets) != 1 || assets[0].Name != "tool_linux_amd64.tar.gz" {
		t.Fatalf("unexpected assets: %+v", assets)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages to be fetched, got %d (%v)", len(pages), pages)
	}
}

func TestConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "installer-config.json")

	if err := SaveConfig(path, &Config{Token: "ghp_secret"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("config perms = %o, want 600", perm)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Token != "ghp_secret" {
		t.Fatalf("token = %q, want ghp_secret", cfg.Token)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, "nope", "installer-config.json"))
	if err != nil {
		t.Fatalf("LoadConfig for missing file: %v", err)
	}
	if cfg.Token != "" {
		t.Fatalf("expected empty token, got %q", cfg.Token)
	}
}
