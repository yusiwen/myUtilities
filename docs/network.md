# network — Network tools

Network diagnostics, HTTP client, port scanning, and file downloads. DNS lookup,
DIG, WHOIS lookup, curl-like HTTP client, port scan (local listener list +
remote TCP probe), and a multi-threaded resumable downloader. Supports both CLI
and web UI (the downloader is CLI-only).

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

# Download (multi-threaded, resumable)
mu network download https://example.com/big.iso
mu network download https://example.com/big.iso -n 8 -o ~/Downloads/
mu network download https://example.com/big.iso --limit-rate 5M --sha256 <hex>
mu network download https://example.com/big.iso --json

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
mu network --server                       # Shortcut for the default port 8091
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

## `mu network download` — Resumable multi-threaded downloader

Downloads an HTTP(S) file with several parallel connections, resumes an
interrupted transfer automatically, and shows live progress (bar, speed, ETA and
the number of working connections).

```
big.iso  45.2%  ████████████░░░░░░░░  452.1 MiB/1000.0 MiB  24.3 MiB/s  ETA 00:22  conn 6/8
```

### Download Flags

| Flag | Description |
|---|---|
| `[url]` | Remote file to fetch (HTTP/HTTPS). |
| `-o`, `--output` | Output file or directory. Default: the remote filename (from `Content-Disposition` or the URL path) in the current directory. |
| `-n`, `--threads` | Parallel connections (default `4`, max `32`). `-n 1` still supports resume. |
| `--no-resume` | Discard the partial file and resume state, then start over. |
| `--force-resume` | Resume even when the remote `ETag`/`Last-Modified` changed. |
| `-f`, `--force` | Overwrite an existing output file. |
| `--block-size` | Work-unit size, e.g. `8M` or `1MiB`. Default: derived from the file size (1–32 MiB, ~4 blocks per connection). |
| `--no-preallocate` | Do not preallocate the output file up front. |
| `-k`, `--insecure` | Skip TLS certificate verification. |
| `--cacert` | PEM file whose certificates are added to the system roots (for private/corporate CAs). Public CAs keep working. |
| `-H`, `--header` | Extra request header as `Key: Value` (repeatable). |
| `-A`, `--auth` | Bearer token (`Authorization: Bearer <token>`). |
| `--user` | HTTP basic auth as `user:password`. |
| `--proxy` | HTTP(S) proxy URL. |
| `-t`, `--timeout` | Connect/TLS/response-header timeout (default `30s`). |
| `--idle-timeout` | Abort a connection that stops sending data (default `30s`). |
| `--retries` | Retries per block (default `3`, exponential backoff). |
| `--limit-rate` | Global speed cap, e.g. `5M` or `500k`. Default: unlimited. |
| `--sha256` | Expected SHA-256 checksum; the download fails on mismatch. |
| `--http1` | Force HTTP/1.1 (disable HTTP/2 connection multiplexing). |
| `--no-progress` | Disable the live progress display. |
| `--verbose` | Show one progress line per connection. |
| `-q`, `--quiet` | Only print errors. |
| `--json` | Print the final result as JSON. |

### Download Behavior

- **Block queue** — the file is split into fixed-size blocks; every connection
  pulls the next unfinished block, so a slow connection delays one block instead
  of a whole contiguous chunk. Blocks are written with `WriteAt` into a
  preallocated (sparse) file.
- **Resume** — progress is recorded in `<output>.part.mu-dl.json` and the payload
  in `<output>.part`; the final file is created only after the transfer (and the
  optional checksum) succeed. Re-running the same command continues where it
  stopped, skipping completed blocks. Resume is refused (with a warning and a
  fresh start) when the remote size/`ETag`/`Last-Modified` changed, and also when
  the partial file no longer matches the recorded blocks — either shorter than
  the highest completed block (a truncated or sparse-unaware copy) or larger than
  the remote size (stale bytes appended). Those cases would otherwise publish a
  file with zero-filled holes or trailing junk, so the download restarts from
  scratch. The stale partial is kept as `<output>.part.old`. Completed blocks are
  only recorded after the payload is flushed to disk, so a crash cannot leave
  blocks marked done whose bytes never landed.
- **Servers without range support** — if the server ignores `Range` requests, the
  downloader falls back to a single connection; such transfers cannot be resumed.
- **Unknown total size** — a server can accept ranges while reporting
  `Content-Range: bytes 0-0/*` (no total). The parallel planner needs the size, so
  such a transfer falls back to a single connection and reports the bytes actually
  written instead of a `-1 B` summary; it cannot be resumed either.
- **Interrupt** — Ctrl-C flushes the resume state, keeps the partial file, prints
  a resume hint, and exits with status `130`.
- **Progress** — the live region is drawn on a TTY and cleared afterwards; on a
  non-TTY it prints at most one plain line per second, and `--json` keeps stdout
  machine-readable. Respects `NO_COLOR`.
- **TLS** — server certificates are verified against the platform trust store (macOS/Windows
  use the native verifier, Linux/BSD read the system CA bundle, honouring `SSL_CERT_FILE`/
  `SSL_CERT_DIR`). `--cacert <pem>` adds extra CAs on top of those roots, so internal or
  corporate TLS interception certificates work without giving up verification; `-k` skips
  chain and hostname verification entirely.
- **Checksum** — `--sha256` verifies the finished file; on mismatch the partial
  file and resume state are kept for inspection and the command exits non-zero.

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
