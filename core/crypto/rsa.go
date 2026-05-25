package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

type RSACipher struct{}

func (c *RSACipher) GenerateKey(bits int) (pubPEM, privPEM []byte, err error) {
	if bits < 1024 {
		bits = 1024
	}
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	privBlock := &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	pubBlock := &pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}

	return pem.EncodeToMemory(pubBlock), pem.EncodeToMemory(privBlock), nil
}

func parsePublicKey(pemData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
}

func parsePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("invalid private key PEM")
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPriv, ok := priv.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA private key")
	}
	return rsaPriv, nil
}

func (c *RSACipher) Encrypt(pubPEM, data []byte) ([]byte, error) {
	pub, err := parsePublicKey(pubPEM)
	if err != nil {
		return nil, err
	}
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, data, nil)
}

func (c *RSACipher) Decrypt(privPEM, data []byte) ([]byte, error) {
	priv, err := parsePrivateKey(privPEM)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, data, nil)
}

func (c *RSACipher) Sign(privPEM, data []byte) ([]byte, error) {
	priv, err := parsePrivateKey(privPEM)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	return rsa.SignPSS(rand.Reader, priv, crypto.SHA256, hash[:], nil)
}

func (c *RSACipher) Verify(pubPEM, data, sig []byte) error {
	pub, err := parsePublicKey(pubPEM)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	return rsa.VerifyPSS(pub, crypto.SHA256, hash[:], sig, nil)
}
