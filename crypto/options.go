package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/term"

	corecrypto "github.com/yusiwen/myUtilities/core/crypto"
)

type Options struct {
	Sm4       Sm4Options       `cmd:"" name:"sm4" help:"SM4 encrypt/decrypt."`
	Passwd    PasswdOptions    `cmd:"" name:"passwd" help:"Generate random password."`
	Des       DesOptions       `cmd:"" name:"des" help:"DES encrypt/decrypt."`
	TripleDes TripleDesOptions `cmd:"" name:"3des" help:"Triple DES encrypt/decrypt."`
	Aes       AesOptions       `cmd:"" name:"aes" help:"AES encrypt/decrypt."`
	Rsa       RsaOptions       `cmd:"" name:"rsa" help:"RSA key generation, encrypt/decrypt, sign/verify."`
	Encode    EncodeOptions    `cmd:"" name:"encode" help:"Encode data (base64/hex/url)."`
	Decode    DecodeOptions    `cmd:"" name:"decode" help:"Decode data (base64/hex/url)."`
	Serve     ServeOptions     `cmd:"" name:"serve" help:"Start crypto toolkit HTTP server."`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8087"`
}

type Sm4Options struct {
	corecrypto.CommonOptions `embed:""`
	Mode                     string `name:"mode" enum:"ecb,cbc" default:"ecb" help:"Cipher mode."`
}

type DesOptions struct {
	corecrypto.CommonOptions `embed:""`
	Mode                     string `name:"mode" enum:"ecb,cbc" default:"ecb" help:"Cipher mode."`
}

type TripleDesOptions struct {
	corecrypto.CommonOptions `embed:""`
	Mode                     string `name:"mode" enum:"ecb,cbc" default:"ecb" help:"Cipher mode."`
}

type AesOptions struct {
	corecrypto.CommonOptions `embed:""`
	Mode                     string `name:"mode" enum:"ecb,cbc" default:"ecb" help:"Cipher mode."`
}

type PasswdOptions struct {
	Length int `short:"l" name:"length" default:"32" help:"Password length (min 8)."`
}

type RsaOptions struct {
	GenKey     RsaGenKeyOptions  `cmd:"" name:"gen-key" help:"Generate RSA key pair."`
	EncryptCmd RsaEncryptOptions `cmd:"" name:"encrypt" help:"RSA encrypt."`
	DecryptCmd RsaDecryptOptions `cmd:"" name:"decrypt" help:"RSA decrypt."`
	SignCmd    RsaSignOptions    `cmd:"" name:"sign" help:"RSA sign."`
	VerifyCmd  RsaVerifyOptions  `cmd:"" name:"verify" help:"RSA verify signature."`
	CertCmd    RsaCertOptions    `cmd:"" name:"cert" help:"Generate self-signed RSA certificate."`
}

type RsaGenKeyOptions struct {
	Bits    int    `name:"bits" default:"2048" help:"Key size in bits (min 1024)."`
	PubOut  string `name:"pub-out" required:"" help:"Path to write public key PEM."`
	PrivOut string `name:"priv-out" required:"" help:"Path to write private key PEM."`
}

type RsaEncryptOptions struct {
	PubKeyFile string `name:"pub-key-file" required:"" help:"Path to public key PEM file."`
	Input      string `name:"input" help:"Input data string." xor:"input"`
	InputFile  string `name:"input-file" help:"Path to input file." xor:"input"`
	OutputFile string `name:"output-file" help:"Path to output file."`
}

type RsaDecryptOptions struct {
	PrivKeyFile string `name:"priv-key-file" required:"" help:"Path to private key PEM file."`
	Input       string `name:"input" help:"Input data string (hex encoded)." xor:"input"`
	InputFile   string `name:"input-file" help:"Path to input file." xor:"input"`
	OutputFile  string `name:"output-file" help:"Path to output file."`
}

type RsaSignOptions struct {
	PrivKeyFile string `name:"priv-key-file" required:"" help:"Path to private key PEM file."`
	Input       string `name:"input" help:"Input data string." xor:"input"`
	InputFile   string `name:"input-file" help:"Path to input file." xor:"input"`
	OutputFile  string `name:"output-file" help:"Path to output file (hex signature)."`
}

type RsaVerifyOptions struct {
	PubKeyFile    string `name:"pub-key-file" required:"" help:"Path to public key PEM file."`
	Input         string `name:"input" help:"Input data string." xor:"input"`
	InputFile     string `name:"input-file" help:"Path to input file." xor:"input"`
	Signature     string `name:"signature" help:"Hex-encoded signature." xor:"sig"`
	SignatureFile string `name:"signature-file" help:"Path to signature file." xor:"sig"`
}

type RsaCertOptions struct {
	CN      string `name:"cn" required:"" help:"Common Name (e.g. localhost, example.com)."`
	SAN     string `name:"san" help:"Subject Alternative Names, comma-separated (e.g. '*.local,192.168.1.1')."`
	Org     string `name:"org" help:"Organization name."`
	Days    int    `name:"days" default:"365" help:"Validity in days."`
	Bits    int    `name:"bits" default:"2048" help:"Key size in bits (min 1024)."`
	CA      bool   `name:"ca" help:"Generate a CA certificate instead of a TLS server certificate."`
	CertOut string `name:"cert-out" default:"cert.pem" help:"Certificate output path."`
	KeyOut  string `name:"key-out" default:"key.pem" help:"Private key output path."`
}

type EncodeOptions struct {
	Type  string `enum:"base64,base64url,hex,url" default:"base64" help:"Encoding type."`
	Input string `arg:"" optional:"" name:"input" help:"Text to encode (or pipe from stdin)."`
}

type DecodeOptions struct {
	Type  string `enum:"base64,base64url,hex,url" default:"base64" help:"Encoding type."`
	Input string `arg:"" optional:"" name:"input" help:"Text to decode (or pipe from stdin)."`
}

func (o *PasswdOptions) Run() error {
	pw, err := corecrypto.GeneratePassword(o.Length)
	if err != nil {
		return err
	}
	fmt.Print(pw)
	return nil
}

func (o *Sm4Options) Run() error {
	return corecrypto.RunCipher(&corecrypto.SM4Cipher{}, &o.CommonOptions, corecrypto.CipherMode(o.Mode))
}

func (o *DesOptions) Run() error {
	return corecrypto.RunCipher(&corecrypto.DESCipher{}, &o.CommonOptions, corecrypto.CipherMode(o.Mode))
}

func (o *TripleDesOptions) Run() error {
	return corecrypto.RunCipher(&corecrypto.TripleDESCipher{}, &o.CommonOptions, corecrypto.CipherMode(o.Mode))
}

func (o *AesOptions) Run() error {
	return corecrypto.RunCipher(&corecrypto.AESCipher{}, &o.CommonOptions, corecrypto.CipherMode(o.Mode))
}

func (o *EncodeOptions) Run() error {
	input, err := resolveInput(o.Input)
	if err != nil {
		return err
	}
	result, err := encode(o.Type, input)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	fmt.Print(result)
	return nil
}

func (o *DecodeOptions) Run() error {
	input, err := resolveInput(o.Input)
	if err != nil {
		return err
	}
	result, err := decode(o.Type, input)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	fmt.Print(result)
	return nil
}

func (o *RsaGenKeyOptions) Run() error {
	rsa := &corecrypto.RSACipher{}
	pub, priv, err := rsa.GenerateKey(o.Bits)
	if err != nil {
		return err
	}
	if err := corecrypto.WriteFile(o.PubOut, pub, 0644); err != nil {
		return err
	}
	if err := corecrypto.WriteFile(o.PrivOut, priv, 0600); err != nil {
		return err
	}
	fmt.Printf("RSA key pair generated (%d bits)\n  public:  %s\n  private: %s\n", o.Bits, o.PubOut, o.PrivOut)
	return nil
}

func (o *RsaEncryptOptions) Run() error {
	pubPEM, err := corecrypto.ReadFile(o.PubKeyFile)
	if err != nil {
		return err
	}
	data, err := resolveInputData(o.Input, o.InputFile)
	if err != nil {
		return err
	}
	enc, err := (&corecrypto.RSACipher{}).Encrypt(pubPEM, data)
	if err != nil {
		return err
	}
	return writeOutput(enc, o.OutputFile)
}

func (o *RsaDecryptOptions) Run() error {
	privPEM, err := corecrypto.ReadFile(o.PrivKeyFile)
	if err != nil {
		return err
	}
	data, err := resolveInputData(o.Input, o.InputFile)
	if err != nil {
		return err
	}
	dec, err := (&corecrypto.RSACipher{}).Decrypt(privPEM, data)
	if err != nil {
		return err
	}
	return writeOutput(dec, o.OutputFile)
}

func (o *RsaSignOptions) Run() error {
	privPEM, err := corecrypto.ReadFile(o.PrivKeyFile)
	if err != nil {
		return err
	}
	data, err := resolveInputData(o.Input, o.InputFile)
	if err != nil {
		return err
	}
	sig, err := (&corecrypto.RSACipher{}).Sign(privPEM, data)
	if err != nil {
		return err
	}
	return writeOutput(sig, o.OutputFile)
}

func (o *RsaVerifyOptions) Run() error {
	pubPEM, err := corecrypto.ReadFile(o.PubKeyFile)
	if err != nil {
		return err
	}
	data, err := resolveInputData(o.Input, o.InputFile)
	if err != nil {
		return err
	}
	sig, err := resolveHexOrFile(o.Signature, o.SignatureFile)
	if err != nil {
		return err
	}
	if err := (&corecrypto.RSACipher{}).Verify(pubPEM, data, sig); err != nil {
		return err
	}
	fmt.Println("Signature verified OK")
	return nil
}

func (o *RsaCertOptions) Run() error {
	if err := requireNotExist(o.CertOut); err != nil {
		return err
	}
	if err := requireNotExist(o.KeyOut); err != nil {
		return err
	}

	var sans []string
	if o.SAN != "" {
		sans = strings.Split(o.SAN, ",")
	}

	params := corecrypto.CertParams{
		CommonName:   o.CN,
		Organization: o.Org,
		SANs:         sans,
		Bits:         o.Bits,
		ValidDays:    o.Days,
		IsCA:         o.CA,
	}

	certPEM, keyPEM, err := (&corecrypto.RSACipher{}).GenerateSelfSignedCert(params)
	if err != nil {
		return err
	}

	if err := os.WriteFile(o.CertOut, certPEM, 0644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(o.KeyOut, keyPEM, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	certType := "server"
	if o.CA {
		certType = "CA"
	}
	fmt.Printf("%s certificate generated (%d bits, %d days)\n  cert: %s\n  key:  %s\n",
		certType, o.Bits, o.Days, o.CertOut, o.KeyOut)
	return nil
}

func requireNotExist(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	return nil
}

func resolveInputData(text, file string) ([]byte, error) {
	if text != "" {
		return []byte(text), nil
	}
	if file != "" {
		return corecrypto.ReadFile(file)
	}
	return io.ReadAll(os.Stdin)
}

func resolveHexOrFile(hexStr, file string) ([]byte, error) {
	if hexStr != "" {
		decoded, err := hex.DecodeString(hexStr)
		if err != nil {
			return nil, fmt.Errorf("invalid hex signature: %w", err)
		}
		return decoded, nil
	}
	if file != "" {
		return corecrypto.ReadFile(file)
	}
	return nil, fmt.Errorf("either --signature or --signature-file is required")
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Crypto toolkit server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/crypto/passwd", handlePasswd)
	mux.HandleFunc("/api/crypto/cipher", handleCipher)
	mux.HandleFunc("/api/crypto/encode", handleEncode)
	mux.HandleFunc("/api/crypto/decode", handleDecode)
}

func handlePasswd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Length int `json:"length"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if req.Length < 8 {
		req.Length = 8
	}
	pw, err := corecrypto.GeneratePassword(req.Length)
	if err != nil {
		http.Error(w, fmt.Sprintf("generate password: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"password": pw})
}

func handleCipher(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Cipher    string `json:"cipher"`
		Mode      string `json:"mode"`
		Op        string `json:"op"`
		Key       string `json:"key"`
		IV        string `json:"iv"`
		Input     string `json:"input"`
		InputHex  bool   `json:"inputHex"`
		OutputHex bool   `json:"outputHex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	var c corecrypto.Cipher
	switch req.Cipher {
	case "aes":
		c = &corecrypto.AESCipher{}
	case "des":
		c = &corecrypto.DESCipher{}
	case "3des":
		c = &corecrypto.TripleDESCipher{}
	case "sm4":
		c = &corecrypto.SM4Cipher{}
	default:
		http.Error(w, "unsupported cipher", http.StatusBadRequest)
		return
	}

	key := padOrTruncate([]byte(req.Key), c.KeySize())
	var iv []byte
	if req.Mode == "cbc" {
		iv = padOrTruncate([]byte(req.IV), c.BlockSize())
	}

	data := []byte(req.Input)
	if req.InputHex {
		d, err := hex.DecodeString(req.Input)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid hex input: %v", err), http.StatusBadRequest)
			return
		}
		data = d
	}

	mode := corecrypto.CipherMode(req.Mode)
	var result []byte
	var err error
	if req.Op == "encrypt" {
		result, err = c.Encrypt(key, iv, data, mode)
	} else {
		result, err = c.Decrypt(key, iv, data, mode)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("%s operation failed: %v", req.Op, err), http.StatusBadRequest)
		return
	}

	out := string(result)
	if req.OutputHex {
		out = hex.EncodeToString(result)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": out})
}

func resolveInput(text string) ([]byte, error) {
	if text != "" {
		return []byte(text), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return io.ReadAll(os.Stdin)
	}
	return nil, fmt.Errorf("input required; pipe input or provide as argument")
}

func encode(typ string, data []byte) (string, error) {
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

func decode(typ string, data []byte) (string, error) {
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

func handleEncode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Type  string `json:"type"`
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	result, err := encode(req.Type, []byte(req.Input))
	if err != nil {
		http.Error(w, fmt.Sprintf("encode: %v", err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func handleDecode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Type  string `json:"type"`
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	result, err := decode(req.Type, []byte(req.Input))
	if err != nil {
		http.Error(w, fmt.Sprintf("decode: %v", err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func padOrTruncate(data []byte, size int) []byte {
	if len(data) < size {
		padded := make([]byte, size)
		copy(padded, data)
		return padded
	}
	return data[:size]
}

func writeOutput(data []byte, file string) error {
	if file != "" {
		return corecrypto.WriteFile(file, data, 0644)
	}
	if _, err := fmt.Print(string(data)); err != nil {
		return err
	}
	return nil
}
