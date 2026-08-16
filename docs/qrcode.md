# qrcode — Generate QR codes

Encode text or file content as a QR code. Output to terminal (Unicode), save as PNG, or
serve via web UI.

```bash
# Terminal output
mu qrcode gen "https://example.com"

# Pipe from stdin
cat xxxx.conf | mu qrcode gen
mu qrcode gen < xxxx.conf

# Save as PNG
mu qrcode gen -o qrcode.png "https://example.com"

# Error correction level
mu qrcode gen --level high "data"

# Serve web UI (standalone)
mu qrcode serve --port 8085
```

Verify decoded content:

```bash
sudo apt install zbar-tools
mu qrcode gen -o /tmp/qr.png "https://example.com"
zbarimg /tmp/qr.png
# QR-Code:https://example.com
```
