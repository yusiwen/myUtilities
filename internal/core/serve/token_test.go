package serve

import (
	"strings"
	"testing"
)

func TestNewTokenIsValidAndUnique(t *testing.T) {
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		token, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if err := ValidateToken(token); err != nil {
			t.Fatalf("NewToken produced an invalid token %q: %v", token, err)
		}
		if len(token) != 16 {
			t.Errorf("token %q has length %d, want 16", token, len(token))
		}
		if seen[token] {
			t.Fatalf("token %q repeated within 64 draws", token)
		}
		seen[token] = true
	}
}

func TestValidateToken(t *testing.T) {
	valid := []string{"abc123", "A-b_c.d~e", "0123456789", strings.Repeat("a", maxTokenLength)}
	for _, token := range valid {
		if err := ValidateToken(token); err != nil {
			t.Errorf("ValidateToken(%q) = %v, want nil", token, err)
		}
	}

	invalid := []string{"", "has space", "with/slash", "with?query", "with#frag", "with%25", "中文", strings.Repeat("a", maxTokenLength+1)}
	for _, token := range invalid {
		if err := ValidateToken(token); err == nil {
			t.Errorf("ValidateToken(%q) = nil, want an error", token)
		}
	}
}
