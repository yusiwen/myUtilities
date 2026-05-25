package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
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

func TestRSAGenerateSelfSignedCert(t *testing.T) {
	c := &RSACipher{}
	params := CertParams{
		CommonName: "localhost",
		Bits:       2048,
		ValidDays:  1,
		SANs:       []string{"localhost", "*.local", "127.0.0.1"},
	}

	certPEM, keyPEM, err := c.GenerateSelfSignedCert(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("empty cert or key")
	}
	if !strings.Contains(string(certPEM), "-----BEGIN CERTIFICATE-----") {
		t.Fatal("cert PEM header missing")
	}
	if !strings.Contains(string(keyPEM), "-----BEGIN PRIVATE KEY-----") {
		t.Fatal("private key PEM header missing")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("invalid cert PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	if cert.Subject.CommonName != "localhost" {
		t.Fatalf("expected CN 'localhost', got %q", cert.Subject.CommonName)
	}
	if !cert.NotAfter.After(time.Now()) {
		t.Fatal("cert expired")
	}
	if cert.IsCA {
		t.Fatal("expected non-CA cert")
	}
	if len(cert.DNSNames) < 2 {
		t.Fatal("expected SAN DNS names")
	}
	if len(cert.IPAddresses) < 1 {
		t.Fatal("expected SAN IP addresses")
	}
}

func TestRSAGenerateSelfSignedCACert(t *testing.T) {
	c := &RSACipher{}
	certPEM, _, err := c.GenerateSelfSignedCert(CertParams{
		CommonName: "My CA",
		Bits:       2048,
		ValidDays:  7,
		IsCA:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	if !cert.IsCA {
		t.Fatal("expected CA cert")
	}
	if cert.BasicConstraintsValid != true {
		t.Fatal("expected basic constraints")
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
