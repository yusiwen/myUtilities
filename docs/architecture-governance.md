# Architecture Governance

## Rule (from AGENTS.md)

> Command packages (`<cmd>/`) are **CLI wrappers only**. They handle:
> - CLI flag definitions (Kong struct tags)
> - Configuration loading
> - User interaction (prompts, confirmations, colored output)
> - Delegation to core packages
>
> Business logic, API clients, and platform operations belong in `core/` packages,
> exposed as public functions and structs so they can be reused across commands
> or tested independently.

## Compliance Status (2026-07)

### Clean (Compliant)

| Package | CLI Lines | Notes |
|---------|-----------|-------|
| `runner` | 19 | Pure delegation |
| `proxy` | 90 | Delegates to `core/proxy/` |
| `git` | 1013 | Delegates to `core/git/`, `core/openai/`, `core/term/` |
| `ask` | 268 | Delegates to `core/llm/`, `core/openai/`, `core/search/` |
| `watch` | 328 | Delegates to `core/watcher/` |
| `qrcode` | 173 | Thin wrapper around `go-qrcode` |
| `serve` | 82 | Minimal static file server |
| `jarinfo` | 253 | Core parsing in `core/jarinfo/` |
| `gateway` | 365 | Inherently orchestrational |
| `diff` | 165 | Simple stdlib wrapper |
| `misc` | 432 | Thin stdlib wrappers |
| `completion` | 161 | Static shell scripts only |

### Violations (Business Logic in CLI Layer)

Violations are ordered by estimated effort (lines to move).

#### Priority 1 (Critical)

**k8s** — 1976 lines total, ~1800 lines of inline business logic

- `command.go` (988): Secret encode/decode, HTTP handlers for config/resources/describe/secret
- `describe.go` (462): describePod, describeNode, describeDeployment, etc.
- `get.go` (476): listPods, listNodes, listDeployments, etc.
- **Fix:** Move all Kubernetes API logic to `core/k8s/`.

#### Priority 2 (High)

**mock** — 1393 lines total, ~1100 lines of inline business logic

- `dynamic_router.go` (265): Endpoint matching, routing, conditional responses
- `dynamicserver.go` (166): Config loading, template resolution
- `dynamic_admin.go` (242): CRUD admin API handlers
- `condition.go` (210): Condition parsing/evaluation
- `mockserver.go` (179): CSV loading, random data generation, query handler
- `fileserver.go` (94): Upload handler
- **Fix:** Move all to `core/mock/`.

**metrics** — 868 lines total, ~700 lines of inline business logic

- `command.go` (767): Agent collection loop, push-to-server with retry, HTTP server handlers
- **Fix:** Move Agent and HTTP handlers to `core/metrics/`.

**svcreg** — 990 lines total, ~500 lines of inline business logic

- `admin.go` (329): Server process manager, start/stop daemon, PID files, admin API
- `client.go` (99): HTTP client for service/instance discovery
- `config.go` (78): Config load/save
- `embed.go` (141): Proxy API handlers
- **Fix:** Move server lifecycle, client, and config to `core/svcreg/`.

**wol** — 858 lines total, ~400 lines of inline business logic

- `command.go` (556): API handlers for WOL dispatch, alias CRUD, notifications, agent retry logic
- **Fix:** Move API handlers and agent logic to `core/wol/`.

**budget** — 605 lines total, ~500 lines of inline business logic

- `command.go` (432): Provider factory, concurrent balance fetching, print formatting
- **Fix:** Move provider logic and formatting to `core/budget/`.

**es** — 570 lines total, ~340 lines of inline business logic

- `client.go` (93): ES client ping, list indices, search
- `command.go` (250): ServerState config, RegisterHandlers API endpoints
- **Fix:** Move ES client and API handlers to `core/es/`.

**installer** — 496 lines total, ~430 lines of inline business logic

- `command.go` (371): GitHub API client, release asset parsing, checksum verification
- `search.go` (60): DuckDuckGo/Google scraping for repo discovery
- `strings.go` (51): OS/arch detection regexes
- **Fix:** Move all to `core/installer/`.

#### Priority 3 (Moderate)

**network** — 430 lines total, ~320 lines of inline business logic

- `command.go` (381): DNS lookup, DIG-style query, WHOIS lookup, SSL certificate checking
- `core/net/` exists but only handles WOL/interface discovery, not DNS/whois/cert
- **Fix:** Extend `core/net/` or create `core/network/`.

**crypto** — 863 lines total, ~150 lines of inline business logic

- `options.go` (814): JWT decode/verify, encode/decode helpers (base64/hex/url)
- SM4/DES/3DES/AES/passwd properly delegate to `core/crypto/`
- **Fix:** Move JWT and encode/decode logic to `core/crypto/`.

## Enforcement

New packages must follow the rule from the start. The audit should be re-run
before each major release to track progress against violations.
