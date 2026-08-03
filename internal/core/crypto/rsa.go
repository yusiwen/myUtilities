package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
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

type CertParams struct {
	CommonName   string
	Organization string
	SANs         []string
	Bits         int
	ValidDays    int
	IsCA         bool
}

func (c *RSACipher) GenerateSelfSignedCert(params CertParams) (certPEM, keyPEM []byte, err error) {
	if params.Bits < 1024 {
		params.Bits = 1024
	}
	if params.ValidDays < 1 {
		params.ValidDays = 365
	}

	priv, err := rsa.GenerateKey(rand.Reader, params.Bits)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, nil, fmt.Errorf("serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   params.CommonName,
			Organization: orEmpty(params.Organization),
		},
		NotBefore:             now,
		NotAfter:              now.Add(time.Duration(params.ValidDays) * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  params.IsCA,
	}

	if params.IsCA {
		template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	} else {
		template.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		for _, s := range params.SANs {
			s = strings.TrimSpace(s)
			if ip := net.ParseIP(s); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			} else {
				template.DNSNames = append(template.DNSNames, s)
			}
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return certPEM, keyPEM, nil
}

func orEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
