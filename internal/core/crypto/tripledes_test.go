package crypto

import (
	"bytes"
	"testing"
)

func tdEncrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	return (&TripleDESCipher{}).Encrypt(key, iv, data, mode)
}

func tdDecrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	return (&TripleDESCipher{}).Decrypt(key, iv, data, mode)
}

func TestTripleDESEncryptDecryptECB(t *testing.T) {
	key := []byte("0123456789abcdef01234567")
	plain := []byte("3DES ECB test message for roundtrip.")

	enc, err := tdEncrypt(key, nil, plain, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, plain) {
		t.Fatal("encrypted text should differ from plain text")
	}

	dec, err := tdDecrypt(key, nil, enc, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}

func TestTripleDESEncryptDecryptCBC(t *testing.T) {
	key := []byte("0123456789abcdef01234567")
	iv := make([]byte, 8)
	copy(iv, []byte("12345678"))
	plain := []byte("3DES CBC roundtrip test.")

	enc, err := tdEncrypt(key, iv, plain, ModeCBC)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := tdDecrypt(key, iv, enc, ModeCBC)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}
