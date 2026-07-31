package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
)

// Encode encodes data using the given type (base64, base64url, hex, url).
func Encode(typ string, data []byte) (string, error) {
	switch typ {
	case "base64":
		return base64.StdEncoding.EncodeToString(data), nil
	case "base64url":
		return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data), nil
	case "hex":
		return hex.EncodeToString(data), nil
	case "url":
		return url.QueryEscape(string(data)), nil
	}
	return "", fmt.Errorf("unknown encoding type: %s", typ)
}

// Decode decodes data using the given type (base64, base64url, hex, url).
func Decode(typ string, data []byte) (string, error) {
	switch typ {
	case "base64":
		d, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return "", err
		}
		return string(d), nil
	case "base64url":
		d, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(string(data))
		if err != nil {
			return "", err
		}
		return string(d), nil
	case "hex":
		d, err := hex.DecodeString(string(data))
		if err != nil {
			return "", err
		}
		return string(d), nil
	case "url":
		d, err := url.QueryUnescape(string(data))
		if err != nil {
			return "", err
		}
		return d, nil
	}
	return "", fmt.Errorf("unknown encoding type: %s", typ)
}

func PadOrTruncate(data []byte, size int) []byte {
	if len(data) < size {
		padded := make([]byte, size)
		copy(padded, data)
		return padded
	}
	return data[:size]
}
