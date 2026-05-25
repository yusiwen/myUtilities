package crypto

import (
	"crypto/rand"
	"math/big"
)

const (
	DefaultPasswordLength = 32
	MinPasswordLength     = 8
	passwordCharset       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

func GeneratePassword(length int) (string, error) {
	if length < MinPasswordLength {
		length = MinPasswordLength
	}

	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordCharset))))
		if err != nil {
			return "", err
		}
		result[i] = passwordCharset[idx.Int64()]
	}
	return string(result), nil
}
