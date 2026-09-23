package es

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestCorruptConfigStillServes covers the nil-config panic: when es-config.json
// exists but cannot be parsed, the API handlers used to dereference a nil
// *Config on every request.
func TestCorruptConfigStillServes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "es-config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewServerState(path)
	if err := state.LoadConfig(); err == nil {
		t.Fatal("expected LoadConfig to reject the corrupt file")
	}
	if state.GetConfig() == nil {
		t.Fatal("GetConfig returned nil after a failed load")
	}

	mux := http.NewServeMux()
	RegisterHandlers(mux, state)
	for _, p := range []string{"/api/status", "/api/config"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", p, rec.Code)
		}
	}
}

// TestMaskedConfigNil documents the defensive nil guard.
func TestMaskedConfigNil(t *testing.T) {
	if got := MaskedConfig(nil); got == nil {
		t.Fatal("MaskedConfig(nil) returned nil")
	}
}
