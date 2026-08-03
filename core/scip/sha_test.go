package scip

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadVerifiedGoodSHA(t *testing.T) {
	content := []byte("hello indexer")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	sum := sha256.Sum256(content)
	good := hex.EncodeToString(sum[:])
	dest := filepath.Join(t.TempDir(), "bin")
	tc := &Toolchain{}
	if err := tc.downloadVerified(srv.URL, good, dest); err != nil {
		t.Fatalf("expected good checksum to pass, got %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != string(content) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadVerifiedBadSHA(t *testing.T) {
	content := []byte("hello indexer")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	tc := &Toolchain{}
	err := tc.downloadVerified(srv.URL, "deadbeef", dest)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha256 mismatch error, got %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatalf("corrupted download should not leave the destination file")
	}
}

func TestDownloadVerifiedNoSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin")
	tc := &Toolchain{}
	// empty checksum → verification skipped, download still succeeds
	if err := tc.downloadVerified(srv.URL, "", dest); err != nil {
		t.Fatalf("empty checksum should skip verification, got %v", err)
	}
}
