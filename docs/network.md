# network — Network tools

DNS lookup, DIG (detailed DNS query), and WHOIS lookup. Supports both CLI and web UI.

```bash
# DNS lookup
mu network dns example.com                # A record (default)
mu network dns example.com --type MX      # MX record
mu network dns example.com --type ALL     # All record types

# DIG (detailed query with full response)
mu network dig example.com                # dig-style output
mu network dig example.com --type MX
mu network dig example.com -n 8.8.8.8     # Specify nameserver

# WHOIS lookup
mu network whois example.com

# Serve web UI (standalone)
mu network serve --port 8091
```

The web UI provides:
- **DNS Lookup** tab — query various record types with TTL display
- **DIG** tab — full dig-style output with response headers, sections, and timing
- **WHOIS** tab — domain WHOIS lookup
