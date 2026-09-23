package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestQueryHandlerPagination covers the pagination contract: a missing or
// out-of-range pageSize must never divide by zero (that panicked the handler
// with "integer divide by zero"), and a page beyond the last one returns an
// empty result instead of an out-of-range slice.
func TestQueryHandlerPagination(t *testing.T) {
	srv, err := NewMockServer(25, "")
	if err != nil {
		t.Fatalf("NewMockServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	tests := []struct {
		name     string
		body     string
		wantRows int // -1: only require a JSON array
	}{
		{name: "empty body uses the default page size", body: `{}`, wantRows: -1},
		{name: "pageNo only", body: `{"pageNo":1}`, wantRows: -1},
		{name: "zero pageSize", body: `{"pageNo":1,"pageSize":0}`, wantRows: -1},
		{name: "negative pageSize", body: `{"pageNo":1,"pageSize":-5}`, wantRows: -1},
		{name: "small explicit page", body: `{"pageNo":1,"pageSize":7}`, wantRows: 7},
		{name: "second page", body: `{"pageNo":2,"pageSize":10}`, wantRows: 10},
		{name: "absurd pageSize is clamped", body: `{"pageNo":1,"pageSize":2000000000}`, wantRows: -1},
		{name: "page beyond the last is empty", body: `{"pageNo":9999,"pageSize":10}`, wantRows: 0},
		{name: "negative pageNo is clamped to one", body: `{"pageNo":-3,"pageSize":5}`, wantRows: 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(ts.URL+"/api/mock/query/default", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			var out struct {
				Status struct {
					Code string `json:"Code"`
				} `json:"Status"`
				Result struct {
					Data []any `json:"Data"`
				} `json:"Result"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.Status.Code != "0" {
				t.Fatalf("status code = %q, want \"0\"", out.Status.Code)
			}
			if out.Result.Data == nil {
				t.Fatal("Data is nil; want a JSON array")
			}
			if tc.wantRows >= 0 && len(out.Result.Data) != tc.wantRows {
				t.Fatalf("rows = %d, want %d", len(out.Result.Data), tc.wantRows)
			}
		})
	}

	// Keep the invariants that prevent the panic explicit.
	if defaultPageSize <= 0 {
		t.Fatalf("defaultPageSize = %d, want > 0", defaultPageSize)
	}
	if maxPageSize < defaultPageSize {
		t.Fatalf("maxPageSize = %d, want >= %d", maxPageSize, defaultPageSize)
	}
}
