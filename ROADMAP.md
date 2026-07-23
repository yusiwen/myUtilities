# Roadmap

Potential features for future consideration, ordered by implementation priority.

## Status

| # | Feature | Status |
|---|---------|--------|
| 1 | `mu serve` — Static file server | ✅ Done |
| 2 | `mu ask` — LLM Q&A with web search | ✅ Done |

## Proposed Features

### 2. `mu svcreg` — ServiceCenter-compatible service registry

✅ Done — v1.2.8 implements a lightweight BoltDB-backed server compatible with
the Apache ServiceComb ServiceCenter v4 REST protocol. Supports service/instance
registration, heartbeat (REST + WS), service discovery, tag/schema management,
WebSocket watcher, environment-based isolation, Svelte 5 web dashboard with admin
server lifecycle management (start/stop with PID file recovery), and independent
process group for gateway restart safety.

### 4. `mu encode` — Encoding/decoding toolbox

Quick inline base64, URL, hex encoding/decoding and JWT payload decoding. Useful for
daily ad-hoc debugging.

```
mu encode base64 "hello"
mu encode base64 -d "aGVsbG8="
mu encode jwt <token>
```

**Depends on:** `encoding/base64`, `encoding/hex`, `net/url`, `encoding/json` (all stdlib)
**Complexity:** Low (single file, ~100 lines)

---

### 3. `mu tail` — File tail / log follower

Exposes the existing `core/watcher.FileWatcher` as a CLI command. Follows a file
and prints new lines as they are written.

```
mu tail app.log
mu tail --lines 50 app.log
```

**Depends on:** `core/watcher` (already exists)
**Complexity:** Low (thin CLI wrapper over existing core package)

---

### 4. `mu cert` — Certificate inspector

Reads a PEM certificate file and displays subject, issuer, expiry date, and SANs.
Complements `crypto rsa cert` (generation) with inspection.

```
mu cert info server.pem
```

**Depends on:** `crypto/x509` (stdlib)
**Complexity:** Low (single file, ~80 lines)

---

### 5. `mu port` — TCP connectivity check

Simple TCP dial to check if a remote port is open, with response time.

```
mu port check db.example.com:5432
```

**Depends on:** `net` (stdlib)
**Complexity:** Low (single file, ~50 lines)
