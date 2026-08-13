package misc

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
)

// Hash computes the hex digest of input. Supported algorithms: md5, sha256,
// sha512. Any other value returns an error.
func Hash(alg, input string) (string, error) {
	var h hash.Hash
	switch alg {
	case "md5":
		h = md5.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return "", fmt.Errorf("unsupported algorithm: %s", alg)
	}
	h.Write([]byte(input))
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
