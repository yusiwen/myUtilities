package crypto

import (
	"bytes"
	"testing"
)

func aesEncrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	return (&AESCipher{}).Encrypt(key, iv, data, mode)
}

func aesDecrypt(key, iv, data []byte, mode CipherMode) ([]byte, error) {
	return (&AESCipher{}).Decrypt(key, iv, data, mode)
}

func TestAESEncryptDecryptECB(t *testing.T) {
	key := make([]byte, 16)
	copy(key, []byte("0123456789abcdef"))
	plain := []byte("Hello, AES ECB test message for roundtrip.")

	enc, err := aesEncrypt(key, nil, plain, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(enc, plain) {
		t.Fatal("encrypted text should differ from plain text")
	}

	dec, err := aesDecrypt(key, nil, enc, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}

func TestAESEncryptDecryptCBC(t *testing.T) {
	key := make([]byte, 32)
	copy(key, []byte("012345678901234567890123456789ab"))
	iv := make([]byte, 16)
	copy(iv, []byte("abcdefghijklmnop"))
	plain := []byte("AES-256 CBC roundtrip test.")

	enc, err := aesEncrypt(key, iv, plain, ModeCBC)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := aesDecrypt(key, iv, enc, ModeCBC)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}

func TestAESKeySizes(t *testing.T) {
	plain := []byte("test")
	for _, size := range []int{16, 24, 32} {
		key := make([]byte, size)
		enc, err := aesEncrypt(key, nil, plain, ModeECB)
		if err != nil {
			t.Fatalf("key size %d: encrypt failed: %v", size, err)
		}
		dec, err := aesDecrypt(key, nil, enc, ModeECB)
		if err != nil {
			t.Fatalf("key size %d: decrypt failed: %v", size, err)
		}
		if !bytes.Equal(dec, plain) {
			t.Fatalf("key size %d: roundtrip failed", size)
		}
	}
}

func TestAESInvalidKeySize(t *testing.T) {
	key := make([]byte, 17)
	_, err := aesEncrypt(key, nil, []byte("data"), ModeECB)
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
}

func TestAESMultiBlock(t *testing.T) {
	key := make([]byte, 16)
	plain := make([]byte, 1000)
	for i := range plain {
		plain[i] = byte(i % 256)
	}

	enc, err := aesEncrypt(key, nil, plain, ModeECB)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := aesDecrypt(key, nil, enc, ModeECB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatal("multi-block roundtrip failed")
	}
}
