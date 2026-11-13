package serve

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// maxTokenLength bounds the token so the download URL stays short enough for a
// small, quick-to-scan QR code.
const maxTokenLength = 64

// NewToken returns a random URL-safe token for a download link. Twelve random
// bytes give 96 bits of entropy and a 16-character token.
func NewToken() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ValidateToken rejects tokens that cannot be used unchanged as a URL path
// segment. Only the RFC 3986 unreserved characters are accepted, which keeps
// the token readable in a QR code and safe to copy from a browser.
func ValidateToken(token string) error {
	if token == "" {
		return fmt.Errorf("token must not be empty")
	}
	if len(token) > maxTokenLength {
		return fmt.Errorf("token is %d characters, want at most %d", len(token), maxTokenLength)
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '.', r == '_', r == '~':
		default:
			return fmt.Errorf("token contains %q; use only letters, digits and - . _ ~", r)
		}
	}
	return nil
}
