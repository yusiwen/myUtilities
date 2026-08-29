# http — HTTP client

A lightweight curl-like HTTP client for sending requests and inspecting
responses. Designed for quick API debugging: it auto-formats JSON, follows
redirects by default, and prints a one-line summary (method, URL, status,
latency) to stderr so the body can be cleanly piped or redirected.

The command is available under both `mu http` and `mu network http`.

```bash
# GET a URL
mu http https://api.example.com/users
mu network http https://api.example.com/users

# POST a JSON body
mu http -X POST -d '{"name":"demo"}' https://api.example.com/users

# Custom headers (repeatable)
mu http -H "X-Token: abc" -H "Accept: application/json" https://api.example.com

# Bearer auth
mu http -A "Bearer token123" https://api.example.com/me

# Force pretty-print for JSON responses
mu http -j https://api.example.com/data

# Pipe a body from stdin
cat payload.json | mu http -X POST https://api.example.com/items

# Write the body to a file instead of stdout
mu http -o response.json https://api.example.com/data

# Body only (no status line or headers)
mu http -b https://api.example.com/data

# Skip TLS verification (self-signed certs)
mu http -k https://self-signed.local/api

# Do not follow redirects
mu http -N https://example.com/redirect
```

## Flags

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

## Behavior

- **Body input** — pass a body with `-d`, or pipe it on stdin when `-d` is not
  set (e.g. `cat payload.json | mu http -X POST <url>`).
- **Content-Type** — when a body is present and no `Content-Type` header was
  supplied, `application/json` is set automatically.
- **Redirects** — followed by default; use `-N` to stop at the first redirect.
- **JSON output** — responses are pretty-printed when the `Content-Type`
  contains `json` or the body looks like a JSON object/array; force it with `-j`.
- **Output** — the status line and headers are printed to stdout with the body;
  a one-line summary (`GET https://… → 200 (5.8s)`) is written to stderr, so
  `mu http … | jq .` still works when combined with `-b` or `-o`.
- **Colors** — the status line is green for 2xx, red for 4xx/5xx. Respects the
  `NO_COLOR` environment variable.
