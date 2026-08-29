# network — Network tools

Network diagnostics and HTTP client. DNS lookup, DIG, WHOIS lookup, and
curl-like HTTP client. Supports both CLI and web UI.

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

# HTTP client (curl-like)
mu network http https://api.example.com/users
mu network http -X POST -d '{"name":"demo"}' https://api.example.com/users
mu network http -A "Bearer token123" -j https://api.example.com/me

# Serve web UI (standalone)
mu network serve --port 8091
```

## `mu network http` — HTTP client

A lightweight curl-like HTTP client for sending requests and inspecting
responses. Designed for quick API debugging: it auto-formats JSON, follows
redirects by default, and prints a one-line summary (method, URL, status,
latency) to stderr so the body can be cleanly piped or redirected.

### HTTP Flags

| Flag | Description |
|---|---|
| `-X`, `--method` | HTTP method (`GET` (default), `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`) |
| `-H`, `--header` | Request header as `Key: Value` (repeatable) |
| `-d`, `--data` | Request body (or pipe from stdin when omitted) |
| `-A`, `--auth` | Bearer token; sets the `Authorization: Bearer <token>` header |
| `-t`, `--timeout` | Request timeout (e.g. `30s`, `2m`, default `30s`) |
| `-k`, `--insecure` | Skip TLS certificate verification |
| `-N`, `--no-follow` | Do not follow redirects |
| `-j`, `--json` | Force pretty-print JSON response |
| `-b`, `--body` | Print only the response body (no status/headers) |
| `-o`, `--output` | Write the response body to a file instead of stdout |

### HTTP Behavior

- **Body input** — pass a body with `-d`, or pipe it on stdin when `-d` is not
  set (e.g. `cat payload.json | mu network http -X POST <url>`).
- **Content-Type** — when a body is present and no `Content-Type` header was
  supplied, `application/json` is set automatically.
- **Redirects** — followed by default; use `-N` to stop at the first redirect.
- **JSON output** — responses are pretty-printed when the `Content-Type`
  contains `json` or the body looks like a JSON object/array; force it with `-j`.
- **Output** — the status line and headers are printed to stdout with the body;
  a one-line summary (`GET https://… → 200 (5.8s)`) is written to stderr, so
  `mu network http … | jq .` still works when combined with `-b` or `-o`.
- **Colors** — the status line is green for 2xx, red for 4xx/5xx. Respects the
  `NO_COLOR` environment variable.

## Web UI

The web UI provides:
- **DNS Lookup** tab — query various record types with TTL display
- **DIG** tab — full dig-style output with response headers, sections, and timing
- **WHOIS** tab — domain WHOIS lookup
