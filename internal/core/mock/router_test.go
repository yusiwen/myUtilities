package mock

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDynamicRouterRejectsOversizedBody guards maxMockBodyBytes: the router
// buffers the whole request body in memory for matching (and for --verbose
// replay), so an unbounded read would let one request exhaust the process.
func TestDynamicRouterRejectsOversizedBody(t *testing.T) {
	router := NewDynamicRouter([]*ManagedEndpoint{{
		ID:     "e1",
		Method: http.MethodPost,
		Path:   "/api/x",
		Status: http.StatusOK,
		Body:   "ok",
	}}, nil, false)

	small := httptest.NewRequest(http.MethodPost, "/api/x", strings.NewReader(`{"a":1}`))
	small.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, small)
	if rec.Code != http.StatusOK {
		t.Fatalf("small body: got %d, want 200", rec.Code)
	}

	big := httptest.NewRequest(http.MethodPost, "/api/x", bytes.NewReader(make([]byte, maxMockBodyBytes+1)))
	big.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: got %d, want 413", rec.Code)
	}
}
