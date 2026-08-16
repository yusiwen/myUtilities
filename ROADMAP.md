# Roadmap

Potential features for future consideration, ordered by implementation priority.

## Status

| # | Feature | Status |
|---|---------|--------|
| 1 | `mu serve` — Static file server | ✅ Done |
| 2 | `mu ask` — LLM Q&A with web search | ✅ Done |
| 3 | `mu budget` — LLM API balance & usage tracking | ✅ Done |
| 4 | `mu svcreg` — ServiceCenter-compatible service registry | ✅ Done |
| 5 | `mu set` — Unified config via `ModuleSetter` registry | ✅ Done |
| 6 | `mu git review` — AI code review agent | ✅ Done |
| 7 | `mu scip` — SCIP semantic code intelligence | ✅ Done |
| 8 | `mu metrics` — Time-series metrics collection | ✅ Done |
| 9 | `mu encode` / `mu cert` | ✅ Done (via `mu crypto` / `mu network`) |
| 10 | `mu tail` — File tail / log follower | ⬜ Not implemented |
| 11 | `mu port` — TCP connectivity check | ⬜ Not implemented |
| 12 | `mu secret` — OS keyring credential storage | 📋 Planned (see [docs/plan/keyring-module-plan.md](./docs/plan/keyring-module-plan.md)) |

## Proposed Features

### 2. `mu svcreg` — ServiceCenter-compatible service registry

✅ Done — v1.2.8 implements a lightweight BoltDB-backed server compatible with
the Apache ServiceComb ServiceCenter v4 REST protocol. Supports service/instance
registration, heartbeat (REST + WS), service discovery, tag/schema management,
WebSocket watcher, environment-based isolation, Svelte 5 web dashboard with admin
server lifecycle management (start/stop with PID file recovery), and independent
process group for gateway restart safety.

### 4. `mu encode` — Encoding/decoding toolbox

✅ Done — implemented as `mu crypto encode/decode` (base64, base64url, hex, URL)
and `mu crypto jwt decode/verify`. See README → crypto section.

---

### 4. `mu cert` — Certificate inspector

✅ Done — implemented as `mu network cert` (fetch and display SSL/TLS certificate
details for a domain) and `mu crypto rsa cert` (self-signed cert generation).
See README → network section.

---

### 3. `mu tail` — File tail / log follower

Exposes the existing `internal/core/watcher.FileWatcher` as a CLI command. Follows a file
and prints new lines as they are written.

```
mu tail app.log
mu tail --lines 50 app.log
```

**Depends on:** `internal/core/watcher` (already exists)
**Complexity:** Low (thin CLI wrapper over existing core package)
**Status:** ⬜ Not implemented

---

### 5. `mu port` — TCP connectivity check

Simple TCP dial to check if a remote port is open, with response time.

```
mu port check db.example.com:5432
```

**Depends on:** `net` (stdlib)
**Complexity:** Low (single file, ~50 lines)
**Status:** ⬜ Not implemented

---

### 6. `mu secret` — Secure credential storage via OS keyring

Store API keys and credentials in the OS-native keychain/keyring instead of
plain-text config files. Provides a `mu secret` subcommand for CRUD operations
and integrates with `budget`, `commit`, and `ask` modules for transparent
credential resolution.

```
mu secret set deepseek sk-xxx
mu secret get deepseek
mu secret list
mu secret delete deepseek
```

Resolution priority: `--key` flag → config file → OS keyring.

**Keyring backends:** Secret Service (Linux), Keychain (macOS), Credential
Manager (Windows) via `github.com/zalando/go-keyring`.

**Integration points:**
- New `internal/core/config.ModuleSetter` (`mu set`) command provides a unified interface
- `internal/core/secret/keyring.go` — `Set(service, key, value)`, `Get(service, key)`, `Delete(service, key)`
- `internal/budget/config.go` — `resolveAPIKey()` fallback to keyring
- `internal/git/commit.go` — API key resolution fallback
- `internal/ask/command.go` — API key resolution fallback

**Depends on:** `github.com/zalando/go-keyring` (pure Go, no CGO)
**Complexity:** Medium (~200 lines core + ~100 lines CLI + 3 integration edits)

---

### 7. `mu git review` — AI code review agent

✅ Done — Multi-turn LLM agent using OpenAI-compatible tool calling. The agent
receives a diff stat and file list, then autonomously reads files, searches code,
inspects diffs, and reads function context before producing a structured markdown
review. Rendered via `glamour` and paginated through `less -R`.

Key features:
- 4 tools: `read_file`, `read_diff`, `search_code`, `read_function`
- Shared `git-config.json` with `providers` array (replaces `commit-config.json`)
- Reviews saved to `~/.cache/mu/git_reviews/<project>_<branch>_<timestamp>.md` with YAML front matter
- `mu git review --list` to browse saved reviews
- Automatic config migration from old `commit-config.json`


