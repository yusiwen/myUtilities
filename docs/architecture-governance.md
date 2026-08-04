# Architecture Governance

## Rule (from AGENTS.md)

> Command packages (`<cmd>/`) are **CLI wrappers only**. They handle:
> - CLI flag definitions (Kong struct tags)
> - Configuration loading
> - User interaction (prompts, confirmations, colored output)
> - Delegation to core packages
>
> Business logic, API clients, and platform operations belong in `internal/core/` packages,
> exposed as public functions and structs so they can be reused across commands
> or tested independently.

## Compliance Status (2026-08)

> Updated after the cmd/ + internal/ migration (Aug 2026). The business logic for
> all previously-flagged packages has been moved to `internal/core/`; the CLI
> packages below are now thin wrappers. The violations list is retained for
> historical reference with resolution notes.

### Clean (Compliant)

| Package | CLI Lines | Notes |
|---------|-----------|-------|
| `runner` | 19 | Pure delegation |
| `proxy` | 90 | Delegates to `internal/core/proxy/` |
| `git` | 1013 | Delegates to `internal/core/git/`, `internal/core/openai/`, `internal/core/term/` |
| `ask` | 268 | Delegates to `internal/core/llm/`, `internal/core/openai/`, `internal/core/search/` |
| `watch` | 328 | Delegates to `internal/core/watcher/` |
| `qrcode` | 173 | Thin wrapper around `go-qrcode` |
| `serve` | 82 | Minimal static file server |
| `jarinfo` | 253 | Core parsing in `internal/core/jarinfo/` |
| `gateway` | 365 | Inherently orchestrational |
| `diff` | 165 | Simple stdlib wrapper |
| `misc` | 432 | Thin stdlib wrappers |
| `completion` | 161 | Static shell scripts only |
| `k8s` | 335 | Delegates to `internal/core/k8s/` |
| `mock` | 162 | Delegates to `internal/core/mock/` |
| `metrics` | 407 | Delegates to `internal/core/metrics/` |
| `svcreg` | 381 | Delegates to `internal/core/svcreg/` |
| `wol` | 415 | Delegates to `internal/core/wol/` |
| `budget` | 219 | Delegates to `internal/core/budget/` |
| `es` | 224 | Delegates to `internal/core/es/` |
| `installer` | 85 | Delegates to `internal/core/installer/` |
| `network` | 143 | Delegates to `internal/core/network/` |
| `crypto` | 511 | Delegates to `internal/core/crypto/` |

### Violations (Resolved in Aug 2026 refactor)

Violations are listed with original line counts and the resolution that moved the
business logic to `internal/core/`.

#### Priority 1 (Critical)

**k8s** — 1976 lines total, ~1800 lines of inline business logic

- `command.go` (988): Secret encode/decode, HTTP handlers for config/resources/describe/secret
- `describe.go` (462): describePod, describeNode, describeDeployment, etc.
- `get.go` (476): listPods, listNodes, listDeployments, etc.
- **Resolved:** Moved to `internal/core/k8s/`.

#### Priority 2 (High)

**mock** — 1393 lines total, ~1100 lines of inline business logic

- `dynamic_router.go` (265): Endpoint matching, routing, conditional responses
- `dynamicserver.go` (166): Config loading, template resolution
- `dynamic_admin.go` (242): CRUD admin API handlers
- `condition.go` (210): Condition parsing/evaluation
- `mockserver.go` (179): CSV loading, random data generation, query handler
- `fileserver.go` (94): Upload handler
- **Resolved:** Moved to `internal/core/mock/`.

**metrics** — 868 lines total, ~700 lines of inline business logic

- `command.go` (767): Agent collection loop, push-to-server with retry, HTTP server handlers
- **Resolved:** Moved to `internal/core/metrics/`.

**svcreg** — 990 lines total, ~500 lines of inline business logic

- `admin.go` (329): Server process manager, start/stop daemon, PID files, admin API
- `client.go` (99): HTTP client for service/instance discovery
- `config.go` (78): Config load/save
- `embed.go` (141): Proxy API handlers
- **Resolved:** Moved to `internal/core/svcreg/`.

**wol** — 858 lines total, ~400 lines of inline business logic

- `command.go` (556): API handlers for WOL dispatch, alias CRUD, notifications, agent retry logic
- **Resolved:** Moved to `internal/core/wol/`.

**budget** — 605 lines total, ~500 lines of inline business logic

- `command.go` (432): Provider factory, concurrent balance fetching, print formatting
- **Resolved:** Moved to `internal/core/budget/`.

**es** — 570 lines total, ~340 lines of inline business logic

- `client.go` (93): ES client ping, list indices, search
- `command.go` (250): ServerState config, RegisterHandlers API endpoints
- **Resolved:** Moved to `internal/core/es/`.

**installer** — 496 lines total, ~430 lines of inline business logic

- `command.go` (371): GitHub API client, release asset parsing, checksum verification
- `search.go` (60): DuckDuckGo/Google scraping for repo discovery
- `strings.go` (51): OS/arch detection regexes
- **Resolved:** Moved to `internal/core/installer/`.

#### Priority 3 (Moderate)

**network** — 430 lines total, ~320 lines of inline business logic

- `command.go` (381): DNS lookup, DIG-style query, WHOIS lookup, SSL certificate checking
- `internal/core/net/` exists but only handles WOL/interface discovery, not DNS/whois/cert
- **Resolved:** Created `internal/core/network/`.

**crypto** — 863 lines total, ~150 lines of inline business logic

- `options.go` (814): JWT decode/verify, encode/decode helpers (base64/hex/url)
- SM4/DES/3DES/AES/passwd properly delegate to `internal/core/crypto/`
- **Resolved:** Moved JWT and encode/decode logic to `internal/core/crypto/`.

## Enforcement

New packages must follow the rule from the start. The audit should be re-run
before each major release to track progress against violations.
