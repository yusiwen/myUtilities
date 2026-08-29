# network — Network tools

Network diagnostics, HTTP client, and port scanning. DNS lookup, DIG, WHOIS
lookup, curl-like HTTP client, and port scan (local listener list + remote
TCP probe). Supports both CLI and web UI.

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

# Port scan
mu network port-scan                      # List local TCP/UDP listeners
mu network port-scan -p 8080              # Check if port 8080 is used locally
mu network port-scan -u -p 53            # Include UDP listeners (local)
mu network port-scan 10.0.0.5 -p 22,80   # Remote TCP probe
mu network port-scan 10.0.0.5 -c         # Remote scan of common ports
mu network port-scan 10.0.0.5 -p 1-1024 -w 64  # Remote scan port range, 64 workers
mu network port-scan 10.0.0.5 -a -J      # Show all results as JSON

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

## `mu network port-scan` — Port scanning

Local listener discovery and remote TCP port probing. When no target host is
given, lists locally listening ports; when a target is supplied, performs a
concurrent TCP connection probe against the remote host.

### Port-scan Flags

| Flag | Description |
|---|---|
| `[target]` | Target host (IP or hostname). Omit for local listener list. |
| `-p`, `--ports` | Port specification: single (`8080`), range (`1-100`), or comma-separated mix (`22,80,443`). |
| `-c`, `--common` | Scan a set of common well-known ports (31 ports including SSH, HTTP, HTTPS, MySQL, PostgreSQL, Redis, etc.). |
| `-t`, `--timeout` | Per-probe TCP connect timeout (e.g. `2s`, default `2s`). |
| `-w`, `--workers` | Concurrent probe workers (default `32`, max `128`). |
| `-u`, `--udp` | Include UDP listeners in local scan; remote UDP probe is not supported. |
| `-C`, `--no-color` | Disable colored output. |
| `-a`, `--all` | Show all results including closed/unreachable ports. |
| `-J`, `--json` | Output results as JSON. |

### Port-scan Behavior

- **Local mode** (no target) — lists all listening TCP/UDP sockets on the
  current host. On Linux, reads `/proc/net/{tcp,udp,tcp6,udp6}` and correlates
  inodes with `/proc/<pid>/fd/*`; on macOS/BSD, falls back to
  `lsof -nP -iTCP -iUDP -sTCP:LISTEN`. Each row shows protocol, bound address,
  port, PID, user, and truncated process command.
- **Remote mode** (target given) — performs concurrent TCP connection probes
  using `net.DialTimeout`. The target hostname is resolved once up front; the
  host is not re-resolved per-port. Results are sorted by port.
- **Port specification** — `-p` accepts a single port, a range (`1-1024`), or
  a comma-separated mix (`22,80,443,8000-8010`). `-c` uses a predefined set
  of 31 common service ports.
- **Output** — by default only open ports are shown; use `-a` to also list
  closed/unreachable ports. `-J` emits a JSON array of
  `{"host","port","open","elapsed_ms","error"}` objects.
- **UDP caveat** — UDP port scanning is unreliable without application-layer
  probes; `-u` only affects local listing. Remote mode is TCP-only.

## Web UI

The web UI provides:
- **DNS Lookup** tab — query various record types with TTL display
- **DIG** tab — full dig-style output with response headers, sections, and timing
- **WHOIS** tab — domain WHOIS lookup
