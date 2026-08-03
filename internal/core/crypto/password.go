package crypto

import (
	"crypto/rand"
	"math/big"
)

const (
	DefaultPasswordLength = 32
	MinPasswordLength     = 8
	letterCharset         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitsCharset         = "0123456789"
	specialCharset        = "!@#$%^&*()-_=+[]{}|;:,.<>?/~"
	passwordCharset       = letterCharset + digitsCharset
)

type PasswordOptions struct {
	Length         int
	IncludeDigits  bool
	IncludeSpecial bool
}

func GeneratePassword(length int) (string, error) {
	return GeneratePasswordWithOpts(PasswordOptions{
		Length:         length,
		IncludeDigits:  true,
		IncludeSpecial: false,
	})
}

func GeneratePasswordWithOpts(opts PasswordOptions) (string, error) {
	if opts.Length < MinPasswordLength {
		opts.Length = MinPasswordLength
	}

	charset := letterCharset
	if opts.IncludeDigits {
		charset += digitsCharset
	}
	if opts.IncludeSpecial {
		charset += specialCharset
	}

	result := make([]byte, opts.Length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[idx.Int64()]
	}
	return string(result), nil
}
