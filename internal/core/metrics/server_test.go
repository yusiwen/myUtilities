package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*DB, http.Handler) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	now := time.Now()
	db.Write("cpu.used.percent", map[string]string{"host": "a"}, now, 1.0)
	db.Write("cpu.used.percent", map[string]string{"host": "a"}, now.Add(time.Second), 2.0)

	info := ServerInfo{
		Mode:            ModeServerWithAgent,
		Pid:             1234,
		Version:         "v1.3.6.3",
		ConfigDir:       "/etc/mu",
		ConfigFile:      "/etc/mu/metrics-config.json",
		DBPath:          "/etc/mu/metrics.db",
		Retention:       "7d",
		CompactInterval: "1d",
		CollectInterval: "30s",
		Hostname:        "testbox",
		Port:            8096,
		Agent:           true,
	}
	return db, NewServer(db, info)
}

func TestInfoEndpoint(t *testing.T) {
	db, handler := newTestServer(t)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/metrics/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var info ServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatal(err)
	}
	if info.Mode != ModeServerWithAgent {
		t.Fatalf("mode: got %s", info.Mode)
	}
	if info.ConfigDir != "/etc/mu" || info.DBPath != "/etc/mu/metrics.db" {
		t.Fatalf("config fields wrong: %+v", info)
	}
	if info.Series != 1 || info.Points != 2 {
		t.Fatalf("counts wrong: series=%d points=%d", info.Series, info.Points)
	}
	if info.Port != 8096 || !info.Agent {
		t.Fatalf("port/agent wrong: %+v", info)
	}
	_ = db
}

func TestInfoNotShadowedByName(t *testing.T) {
	_, handler := newTestServer(t)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/metrics/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("info shadowed by {name}: status %d", resp.StatusCode)
	}
	// The {name} wildcard would return a JSON array; info must return an object.
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 || body[0] != '{' {
		t.Fatalf("info response is not a JSON object: %s", string(body))
	}
}

func TestServeStatusPayload(t *testing.T) {
	db, _ := newTestServer(t)
	info := ServerInfo{Mode: ModeServer}
	data, err := ServeStatusPayload(info, db)
	if err != nil {
		t.Fatal(err)
	}
	var got ServerInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Points != 2 || got.Series != 1 {
		t.Fatalf("payload counts wrong: %+v", got)
	}
}

func TestInfoDBFileFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.Write("load.1m", nil, time.Now(), 0.5)

	info := ServerInfo{Mode: ModeServer, DBPath: dbPath, Port: 8096}
	srv := httptest.NewServer(NewServer(db, info))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/metrics/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var got ServerInfo
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.DBSize <= 0 {
		t.Fatalf("db_size not populated: %d", got.DBSize)
	}
	if got.DBModified == "" {
		t.Fatalf("db_modified not populated")
	}
}
