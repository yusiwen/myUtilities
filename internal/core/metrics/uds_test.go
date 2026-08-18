//go:build unix

package metrics

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestServeUDS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.sock")
	u := ServeUDS(path, func() []byte {
		b, _ := json.Marshal(ServerInfo{Mode: ModeServerWithAgent, Pid: 42, Points: 7})
		return b
	})
	if u == nil {
		t.Fatal("ServeUDS returned nil")
	}
	defer u.Close()

	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var info ServerInfo
	if err := json.Unmarshal(buf[:n], &info); err != nil {
		t.Fatalf("unmarshal: %v (raw %q)", err, string(buf[:n]))
	}
	if info.Mode != ModeServerWithAgent || info.Pid != 42 || info.Points != 7 {
		t.Fatalf("unexpected payload: %+v", info)
	}
}

func TestServeUDSDuplicateSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.sock")
	first := ServeUDS(path, func() []byte { return []byte(`{"mode":"server"}`) })
	if first == nil {
		t.Fatal("first ServeUDS returned nil")
	}
	defer first.Close()

	second := ServeUDS(path, func() []byte { return []byte(`{"mode":"server-with-agent"}`) })
	if second != nil {
		t.Fatal("expected duplicate ServeUDS to be skipped")
	}

	// The first server still owns the socket and answers.
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, _ := conn.Read(buf)
	if string(buf[:n]) != `{"mode":"server"}` {
		t.Fatalf("expected first payload, got %q", string(buf[:n]))
	}
}
