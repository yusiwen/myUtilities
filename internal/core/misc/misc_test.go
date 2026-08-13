package misc

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGenUUID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		u, err := GenUUID()
		if err != nil {
			t.Fatalf("GenUUID: %v", err)
		}
		if seen[u] {
			t.Fatalf("duplicate uuid %q", u)
		}
		seen[u] = true
		parts := strings.Split(u, "-")
		if len(parts) != 5 {
			t.Fatalf("unexpected uuid format %q", u)
		}
		if parts[2][0] != '4' {
			t.Fatalf("expected version 4 uuid, got %q", u)
		}
	}
}

func TestJSONOps(t *testing.T) {
	formatted, err := FormatJSON(`{"a":1,"b":[1,2]}`)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if !strings.Contains(formatted, "\n  ") {
		t.Fatalf("expected indented output, got %q", formatted)
	}

	if got, err := ValidateJSON(`{"a":1}`); err != nil || !strings.Contains(got, "Valid") {
		t.Fatalf("ValidateJSON valid: %q, %v", got, err)
	}
	if got, err := ValidateJSON(`{bad`); err != nil || !strings.Contains(got, "Invalid") {
		t.Fatalf("ValidateJSON invalid: %q, %v", got, err)
	}

	minified, err := MinifyJSON(`{ "a" : 1 }`)
	if err != nil {
		t.Fatalf("MinifyJSON: %v", err)
	}
	if minified != `{"a":1}` {
		t.Fatalf("MinifyJSON = %q", minified)
	}

	if _, err := FormatJSON(`{bad`); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConvertTimestamp(t *testing.T) {
	cases := []struct {
		in   string
		unix int64
	}{
		{"1735689600", 1735689600},
		{"2025-01-01", 1735689600},
		{"2025-01-01 00:00:00", 1735689600},
		{"2025-01-01T00:00:00Z", 1735689600},
	}
	for _, c := range cases {
		tm, err := ConvertTimestamp(c.in)
		if err != nil {
			t.Fatalf("ConvertTimestamp(%q): %v", c.in, err)
		}
		if tm.Unix() != c.unix {
			t.Fatalf("ConvertTimestamp(%q) = %d, want %d", c.in, tm.Unix(), c.unix)
		}
	}

	now, err := ConvertTimestamp("")
	if err != nil {
		t.Fatalf("ConvertTimestamp(\"\"): %v", err)
	}
	if d := time.Since(now); d > time.Minute {
		t.Fatalf("empty input should return now, got %v ago", d)
	}

	if _, err := ConvertTimestamp("not-a-time"); err == nil {
		t.Fatal("expected error for unparseable input")
	}
}

func TestHash(t *testing.T) {
	// sha256("hello") known value
	if got, err := Hash("sha256", "hello"); err != nil || got != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("sha256(hello) = %q, %v", got, err)
	}
	if got, err := Hash("md5", "hello"); err != nil || got != "5d41402abc4b2a76b9719d911017c592" {
		t.Fatalf("md5(hello) = %q, %v", got, err)
	}
	if _, err := Hash("nope", "hello"); err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
}

func TestTrackersCache(t *testing.T) {
	calls := 0
	c := NewTrackersCache(time.Minute, func() (string, error) {
		calls++
		return "udp://tracker.example.com/announce", nil
	})

	got, err := c.Get(false)
	if err != nil || got == "" || calls != 1 {
		t.Fatalf("first Get: got=%q calls=%d err=%v", got, calls, err)
	}

	// Second call hits the cache without refetching.
	got, err = c.Get(false)
	if err != nil || calls != 1 {
		t.Fatalf("cached Get: calls=%d err=%v", calls, err)
	}

	// Refresh forces a new fetch.
	if _, err = c.Get(true); err != nil || calls != 2 {
		t.Fatalf("refresh Get: calls=%d err=%v", calls, err)
	}

	// Fetch failure returns an error and leaves the cache untouched.
	failing := NewTrackersCache(time.Minute, func() (string, error) {
		return "", errors.New("boom")
	})
	if _, err := failing.Get(false); err == nil {
		t.Fatal("expected error from failing fetch")
	}
}
