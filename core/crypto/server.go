package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// RegisterHandlers registers the crypto API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/crypto/passwd", handlePasswd)
	mux.HandleFunc("/api/crypto/cipher", handleCipher)
	mux.HandleFunc("/api/crypto/encode", handleEncode)
	mux.HandleFunc("/api/crypto/decode", handleDecode)
	mux.HandleFunc("/api/crypto/jwt/decode", handleJwtDecode)
	mux.HandleFunc("/api/crypto/jwt/verify", handleJwtVerify)
}

func handlePasswd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Length  int   `json:"length"`
		Digits  *bool `json:"digits,omitempty"`
		Special bool  `json:"special"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if req.Length < 8 {
		req.Length = 8
	}
	includeDigits := true
	if req.Digits != nil {
		includeDigits = *req.Digits
	}
	pw, err := GeneratePasswordWithOpts(PasswordOptions{
		Length:         req.Length,
		IncludeDigits:  includeDigits,
		IncludeSpecial: req.Special,
	})
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

	var c Cipher
	switch req.Cipher {
	case "aes":
		c = &AESCipher{}
	case "des":
		c = &DESCipher{}
	case "3des":
		c = &TripleDESCipher{}
	case "sm4":
		c = &SM4Cipher{}
	default:
		http.Error(w, "unsupported cipher", http.StatusBadRequest)
		return
	}

	key := PadOrTruncate([]byte(req.Key), c.KeySize())
	var iv []byte
	if req.Mode == "cbc" {
		iv = PadOrTruncate([]byte(req.IV), c.BlockSize())
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

	mode := CipherMode(req.Mode)
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
	result, err := Encode(req.Type, []byte(req.Input))
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
	result, err := Decode(req.Type, []byte(req.Input))
	if err != nil {
		http.Error(w, fmt.Sprintf("decode: %v", err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func handleJwtDecode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	jwt, err := DecodeJWT(req.Token)
	if err != nil {
		http.Error(w, fmt.Sprintf("decode: %v", err), http.StatusBadRequest)
		return
	}
	var h, p interface{}
	json.Unmarshal(jwt.Header, &h)
	json.Unmarshal(jwt.Payload, &p)
	sigHex := hex.EncodeToString([]byte(jwt.Signature))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"header":       h,
		"payload":      p,
		"signature":    jwt.Signature,
		"signatureHex": sigHex,
	})
}

func handleJwtVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token  string `json:"token"`
		Key    string `json:"key"`
		KeyB64 bool   `json:"keyB64"`
		Alg    string `json:"alg"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if req.Alg == "" {
		req.Alg = DetectAlg(req.Token)
	}

	var keyBytes []byte
	var err error
	if req.KeyB64 {
		keyBytes, err = base64.StdEncoding.DecodeString(req.Key)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid base64 key: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		keyBytes = []byte(req.Key)
	}

	jwt, err := DecodeJWT(req.Token)
	if err != nil {
		http.Error(w, fmt.Sprintf("decode: %v", err), http.StatusBadRequest)
		return
	}
	valid, err := VerifyJWT(req.Token, keyBytes, req.Alg)
	if err != nil {
		http.Error(w, fmt.Sprintf("verify: %v", err), http.StatusBadRequest)
		return
	}

	var h, p interface{}
	json.Unmarshal(jwt.Header, &h)
	json.Unmarshal(jwt.Payload, &p)
	sigHex := hex.EncodeToString([]byte(jwt.Signature))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"header":       h,
		"payload":      p,
		"signature":    jwt.Signature,
		"signatureHex": sigHex,
		"valid":        valid,
	})
}
