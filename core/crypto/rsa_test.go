package crypto

import (
	"bytes"
	"testing"

	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
)

func TestRSAKeyGeneration(t *testing.T) {
	c := &RSACipher{}

	pub, priv, err := c.GenerateKey(1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) == 0 || len(priv) == 0 {
		t.Fatal("empty key output")
	}

	_, _, err = c.GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRSAEncryptDecrypt(t *testing.T) {
	c := &RSACipher{}
	pub, priv, err := c.GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("Hello, RSA-OAEP encrypted message!")
	enc, err := c.Encrypt(pub, plain)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := c.Decrypt(priv, enc)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(dec, plain) {
		t.Fatalf("roundtrip failed: got %q, want %q", dec, plain)
	}
}

func TestRSASignVerify(t *testing.T) {
	c := &RSACipher{}
	pub, priv, err := c.GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("data to sign and verify")
	sig, err := c.Sign(priv, data)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Verify(pub, data, sig); err != nil {
		t.Fatal(err)
	}
}

func TestRSAVerifyWrongSignature(t *testing.T) {
	c := &RSACipher{}
	pub, priv, err := c.GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}

	sig, err := c.Sign(priv, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	err = c.Verify(pub, []byte("wrong"), sig)
	if err == nil {
		t.Fatal("expected verification error for wrong data")
	}
}

func TestRSADecryptWrongPrivateKey(t *testing.T) {
	c := &RSACipher{}
	pub, _, err := c.GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := c.GenerateKey(2048)
	if err != nil {
		t.Fatal(err)
	}

	enc, err := c.Encrypt(pub, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Decrypt(priv2, enc)
	if err == nil {
		t.Fatal("expected decryption error with wrong private key")
	}
}

func TestRSAEncryptTooLarge(t *testing.T) {
	c := &RSACipher{}
	pub, _, err := c.GenerateKey(1024)
	if err != nil {
		t.Fatal(err)
	}

	large := make([]byte, 256)
	_, err = c.Encrypt(pub, large)
	if err == nil {
		t.Fatal("expected error for oversized data")
	}
}

var sinkRSAByte []byte

func BenchmarkRSAGenerateKey2048(b *testing.B) {
	c := &RSACipher{}
	for i := 0; i < b.N; i++ {
		pub, priv, err := c.GenerateKey(2048)
		if err != nil {
			b.Fatal(err)
		}
		sinkRSAByte = pub
		sinkRSAByte = priv
	}
}

func BenchmarkRSAEncrypt1024(b *testing.B) {
	c := &RSACipher{}
	pub, _, err := c.GenerateKey(1024)
	if err != nil {
		b.Fatal(err)
	}
	data := []byte("benchmark data")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := c.Encrypt(pub, data)
		if err != nil {
			b.Fatal(err)
		}
		sinkRSAByte = enc
	}
}

var sinkAESByte []byte

func BenchmarkAESEncryptECB128(b *testing.B) {
	c := &AESCipher{}
	key := make([]byte, 16)
	data := make([]byte, 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := c.Encrypt(key, nil, data, ModeECB)
		if err != nil {
			b.Fatal(err)
		}
		sinkAESByte = enc
	}
}

func BenchmarkAESDecryptECB128(b *testing.B) {
	c := &AESCipher{}
	key := make([]byte, 16)
	enc, _ := c.Encrypt(key, nil, make([]byte, 1024), ModeECB)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec, err := c.Decrypt(key, nil, enc, ModeECB)
		if err != nil {
			b.Fatal(err)
		}
		sinkAESByte = dec
	}
}

var _ = rand.Reader
var _ = rsa.GenerateKey
var _ = sha256.New
