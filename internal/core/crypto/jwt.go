package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
)

// JWT holds the decoded parts of a token.
type JWT struct {
	Header    []byte
	Payload   []byte
	Signature string // raw base64url signature
}

// DecodeJWT splits and base64-decodes a JWT token.
func DecodeJWT(token string) (*JWT, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: expected 3 parts separated by dots")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return &JWT{Header: header, Payload: payload, Signature: parts[2]}, nil
}

// DetectAlg reads the alg header from a JWT token (defaults to HS256).
func DetectAlg(token string) string {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) < 2 {
		return "HS256"
	}
	h, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "HS256"
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	json.Unmarshal(h, &hdr)
	if hdr.Alg == "" {
		return "HS256"
	}
	return hdr.Alg
}

// VerifyJWT verifies an HMAC-signed JWT token.
func VerifyJWT(token string, key []byte, alg string) (bool, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return false, fmt.Errorf("invalid JWT: expected 3 parts")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("decode signature: %w", err)
	}
	var h hash.Hash
	switch strings.ToUpper(alg) {
	case "HS384":
		h = hmac.New(sha512.New384, key)
	case "HS512":
		h = hmac.New(sha512.New, key)
	default:
		h = hmac.New(sha256.New, key)
	}
	signingInput := parts[0] + "." + parts[1]
	h.Write([]byte(signingInput))
	return hmac.Equal(sig, h.Sum(nil)), nil
}
