package crypto

import (
	"bytes"
	"testing"
)

func desEncrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	return (&DESCipher{}).Encrypt(key, iv, data, mode)
}

func desDecrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	return (&DESCipher{}).Decrypt(key, iv, data, mode)
}

func TestDESEncryptDecryptECB(t *testing.T) {
	key := []byte("01234567")
	plain := []byte("Hello DES ECB!")

	enc, err := desEncrypt(key, nil, plain, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, plain) {
		t.Fatal("encrypted text should differ from plain text")
	}

	dec, err := desDecrypt(key, nil, enc, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}

func TestDESEncryptDecryptCBC(t *testing.T) {
	key := []byte("01234567")
	iv := make([]byte, 8)
	copy(iv, []byte("87654321"))
	plain := []byte("DES CBC test message.")

	enc, err := desEncrypt(key, iv, plain, ModeCBC)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := desDecrypt(key, iv, enc, ModeCBC)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}

func TestDESInvalidKeySize(t *testing.T) {
	key := []byte("short")
	_, err := desEncrypt(key, nil, []byte("data"), ModeECB)
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
}
