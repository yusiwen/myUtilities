# crypto — Cryptographic tools

Encrypt and decrypt data with various algorithms (AES, DES, 3DES, SM4), generate
secure random passwords, decode JWT tokens, and encode/decode data. Supports both CLI and web UI.

```bash
# Generate a random password (with options)
mu crypto passwd -l 32
mu crypto passwd -l 16 --no-digits --special

# AES encrypt
mu crypto aes -e --plain-key "mykey" --input "hello" --output-format hex

# AES decrypt
mu crypto aes -d --plain-key "mykey" --input "hex-encoded-data" --input-format hex

# Encode / decode (base64, hex, URL)
mu crypto encode --type base64 "hello"
mu crypto decode --type hex "68656c6c6f"

# JWT decode and verify
mu crypto jwt decode <token>
mu crypto jwt verify --key secret <token>

# Serve web UI (standalone)
mu crypto serve --port 8087
```

The web UI provides:
- **Password Generator** tab — configurable length, digits/special char toggles, one-click copy
- **Encrypt / Decrypt** tab — cipher selection (AES/DES/3DES/SM4), ECB/CBC mode, key/IV input
- **Encode / Decode** tab — base64, base64url, hex, URL encode/decode
- **JWT** tab — decode JWT tokens, verify HMAC signatures with auto-detected algorithm
