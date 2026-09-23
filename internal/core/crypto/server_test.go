package crypto

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPasswordLengthIsCapped verifies that the serve API rejects an oversized
// length instead of allocating (and crypto/rand-filling) whatever the caller
// asks for, and that an oversized body is refused before it is buffered.
func TestPasswordLengthIsCapped(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		name    string
		body    string
		want    int
		wantLen int
	}{
		{name: "normal length", body: `{"length":16}`, want: http.StatusOK, wantLen: 16},
		{name: "below the minimum is clamped", body: `{"length":1}`, want: http.StatusOK, wantLen: 8},
		{
			name:    "at the maximum",
			body:    fmt.Sprintf(`{"length":%d}`, maxPasswordLength),
			want:    http.StatusOK,
			wantLen: maxPasswordLength,
		},
		{
			name: "over the maximum",
			body: fmt.Sprintf(`{"length":%d}`, maxPasswordLength+1),
			want: http.StatusBadRequest,
		},
		{name: "absurd length", body: `{"length":2000000000}`, want: http.StatusBadRequest},
		{
			name: "oversized body",
			body: `{"length":16,"pad":"` + strings.Repeat("x", 70<<10) + `"}`,
			want: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/api/crypto/passwd", "application/json", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("got %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want != http.StatusOK {
				return
			}
			var out struct {
				Password string `json:"password"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(out.Password) != tc.wantLen {
				t.Fatalf("password length = %d, want %d", len(out.Password), tc.wantLen)
			}
		})
	}
}
