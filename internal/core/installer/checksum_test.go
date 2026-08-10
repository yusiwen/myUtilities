package installer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("a694cae143c32c5b6226362fb4bd268a8d13d3cd9b482819b3b0029a9a97b8fe  scip-java-v0.13.1\n"))
	}))
	defer srv.Close()

	if got := (&Client{}).fetchChecksum(srv.URL); got != "a694cae143c32c5b6226362fb4bd268a8d13d3cd9b482819b3b0029a9a97b8fe" {
		t.Fatalf("unexpected checksum: %q", got)
	}
}

func TestFetchChecksumError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if got := (&Client{}).fetchChecksum(srv.URL); got != "" {
		t.Fatalf("expected empty checksum on error, got %q", got)
	}
}
