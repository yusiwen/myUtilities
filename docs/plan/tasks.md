# myUtilities Improvement Tasks

This document contains a list of actionable improvement tasks for the myUtilities project. Each task is marked with a checkbox that can be checked off when completed.

## Code Organization and Structure

> Detailed execution plan: [Codebase Restructure Plan](./codebase-restructure-plan.md) (Phases 1–4)

1. [x] Refactor the project to follow standard Go project layout (cmd/, pkg/, internal/, etc.)
    → See [Phase 3 — Standard Layout](./codebase-restructure-plan.md#phase-3--标准项目布局)
2. [x] Move version information to a dedicated package for better maintainability
    → See [Phase 1-①](./codebase-restructure-plan.md#1-①-版本信息抽到独立包)
    → Done: moved to `internal/core/version` (`cmd/mu/version.go` removed, Makefile ldflags updated)
3. [x] Create separate packages for common utilities instead of embedding them in specific packages
    → See [Phase 2-④](./codebase-restructure-plan.md#2-④-解决-4-组包名冲突)
    → Done: package name conflicts resolved during `cmd/` + `internal/` restructuring
4. [x] Standardize naming conventions across the codebase
    → See [Phase 1-②](./codebase-restructure-plan.md#1-②-修复-pascalcase-文件名) + [Phase 4-⑥](./codebase-restructure-plan.md#4-⑥-统一命名规范)
    → Done: PascalCase filenames fixed during restructuring (proxy, watcher, runner, etc.)
5. [ ] Remove commented-out code and TODOs, replacing them with actual implementations or GitHub issues
    → See [Phase 1-③](./codebase-restructure-plan.md#1-③-清理-todo)

## Documentation

6. [ ] Add comprehensive README.md with installation and usage instructions
7. [ ] Add godoc comments to all exported functions, types, and packages
8. [ ] Create usage examples for each command
9. [ ] Document the build process and release workflow
10. [ ] Add CONTRIBUTING.md with guidelines for contributors

## Testing

11. [ ] Implement unit tests for all packages (current test coverage appears to be minimal or non-existent)
12. [ ] Add integration tests for the installer and mock packages
13. [ ] Set up CI to run tests automatically on pull requests
14. [ ] Implement benchmarks for performance-critical code
15. [ ] Add test mocks for external dependencies (GitHub API, search engines)

## Error Handling

16. [ ] Replace panic with proper error handling in installer/search.go
17. [ ] Standardize error messages and error types across the codebase
18. [ ] Implement structured logging instead of fmt.Printf and log.Println
19. [ ] Add context to errors for better debugging
20. [ ] Improve error reporting to users with more actionable messages

## Performance

21. [ ] Optimize GitHub API requests to reduce rate limiting issues
22. [ ] Implement caching for frequently accessed data
23. [ ] Use goroutines for concurrent operations where appropriate
24. [ ] Profile the application to identify bottlenecks
25. [ ] Optimize memory usage, especially when handling large files

## Security

26. [ ] Implement proper input validation for all user inputs
27. [ ] Sanitize file paths in the mock file server
28. [ ] Add HTTPS support to the mock file server
29. [ ] Implement authentication and authorization for the mock file server
30. [ ] Audit dependencies for security vulnerabilities

## Feature Enhancements

31. [ ] Add Windows support for the installer (currently commented out as TODO)
32. [ ] Implement support for more package formats (deb, rpm, etc.)
33. [ ] Add progress reporting during installations
34. [ ] Implement a configuration file for persistent settings
35. [x] Add more mock services beyond the file server (dynamic-server with admin UI)
36. [ ] **git extensions** — Add `mu git undo` (undo last commit / merge / rebase with safe defaults), `mu git clean --dry-run` (preview untracked file removal), `mu git branches --merged` (list prunable merged branches)
37. [ ] **misc extension: format conversion** — Add `mu misc convert` subcommand: JSON ↔ YAML, CSV ↔ JSON, unit conversion (bytes ↔ human-readable), base64 ↔ hex
38. [ ] **network extension: connectivity** — Add `mu network ping <host>` (ICMP/TCP ping with loss stats) and `mu network trace <host>` (traceroute with per-hop latency) to complement existing `dns`/`dig`/`whois`

## Build and Deployment

39. [ ] Update the Makefile to support all target platforms
40. [x] Implement semantic versioning
41. [ ] Automate the release process completely
42. [ ] Add containerization support (Docker)
43. [ ] Create installation packages for different package managers (apt, brew, etc.)

## User Experience

44. [x] Improve command-line help messages and documentation
45. [ ] Add color and formatting to terminal output
46. [ ] Implement interactive mode for complex operations
47. [x] Add command completion for shells
48. [x] Create a web UI for the mock services

## Web UI Candidates (New Feature Ideas)

Simple tools that would benefit from a web UI and gateway integration:

| Priority | Module | Description | Backend | Frontend | Status |
|---|---|---|---|---|---|
| 🥇 | **JSON Tool** | Format, validate, compress, and query JSON (reuse CodeMirror) | ⭐ ~10 lines | ⭐ ~100 lines | ✅ Done (`mu misc json`) |
| 🥇 | **UUID** | Generate UUID v1/v4/v7, single or batch | ⭐ ~20 lines | ⭐ ~50 lines | ✅ Done (`mu misc uuid`) |
| 🥈 | **Timestamp** | Unix timestamp ↔ human date/time, auto-detect format | ⭐ ~30 lines | ⭐ ~60 lines | ✅ Done (`mu misc timestamp`) |
| 🥈 | **Hash** | File upload or text input → SHA1/SHA256/SHA512/MD5 | ⭐⭐ ~40 lines | ⭐ ~80 lines | ✅ Done (`mu misc hash`) |
| 🥉 | **DNS Lookup** | DNS record queries (A/AAAA/MX/NS/TXT) | ⭐ ~40 lines | ⭐ ~60 lines | ✅ Done (`mu network dns/dig`) |
| 🥉 | **Port Scan** (`mu network port-scan`) | Local TCP/UDP listener listing + remote TCP probe. CLI done; web UI pending. | ⭐⭐ ~60 lines | ⭐ ~80 lines | ✅ CLI done |
| — | **HTTP Client** (`mu network http`) | Web-based curl: method, URL, headers, body → response. Now a subcommand of `mu network`. | ⭐⭐⭐ ~80 lines | ⭐⭐⭐ ~150 lines | ✅ Done (`mu network http`) |
| — | **watch** dashboard | File/git watch events via SSE stream to browser | ⭐⭐ (internal/core/watcher ready) | ⭐⭐⭐ ~200 lines | ☐ |
| — | **git commit** UI | Stage files, view diff, generate/edit commit message via LLM | ⭐⭐ (internal/core/git + openai ready) | ⭐⭐⭐ ~200 lines | ☐ |

## Proposed New Modules

Modules not yet covered by the Web UI candidates above. Each follows the standard pattern: `internal/<name>/` CLI + `internal/core/<name>/` core logic + optional web UI + gateway registration.

| Priority | Module | Description | Effort |
|---|---|---|---|
| 🥇 | **HTTP Client** (`mu network http`) ✅ | curl-like CLI HTTP client folded into `mu network`: `mu network http <url>` with `-X` method, `-H` headers, `-d`/stdin body, `-A` bearer auth, `-t` timeout, `-j` pretty JSON, `-b` body-only, `-o` file output; status line color-coded, one-line summary to stderr. Core in `internal/core/httpclient`; CLI in `internal/network`. See [docs/network.md](./network.md) | ⭐⭐ ~100 lines |
| 🥇 | **Log Tailer** (`mu log`) ✅ | Tail and filter log files: `mu log -f app.log --level error --since 5m --grep "timeout"`. Multi-file tail, JSON-line auto-detection, colorized level highlighting. Core in `internal/core/logtail`; CLI-only (no web UI needed — terminal is the natural interface). See [docs/log.md](./log.md) | ⭐⭐ ~120 lines |
| 🥉 | **Port Scan** (`mu network port-scan`) ✅ | ✅ **Done** — merged into `mu network`: `mu network port-scan` lists local TCP/UDP listeners (PID, user, process); `mu network port-scan <host> -p 22,80` probes remote ports; `-c` for common ports, `-a` for all results, `-J` for JSON. Linux uses `/proc/net` + `/proc/<pid>/fd`; macOS/BSD use `lsof`. Core in `internal/core/network`, CLI in `internal/network`. See [docs/network.md](../network.md). |
| 🥈 | **System Diagnostics** (`mu sys`) | `mu sys check` — one-shot health snapshot: disk usage, memory, load, top-10 processes, network interfaces, DNS resolution. `mu sys env` — dump environment snapshot (Go version, OS, key env vars) for bug reports. Pairs with `fleet` for remote host diagnostics. | ⭐⭐ ~100 lines |
| 🥈 | **Env Manager** (`mu env`) | `mu env list [--filter K]` — list env vars, filtered. `mu env diff prod staging` — diff two env files. `mu env scan` — scan current Go/JS/Python codebase for `os.Getenv`/`process.env`/`os.environ` references and generate a `.env.example` template. | ⭐⭐ ~80 lines |
| 🥉 | **Cron Manager** (`mu cron`) | `mu cron list` — human-readable crontab. `mu cron add "0 9 * * 1" "backup.sh"`. `mu cron next "*/15 * * * *"` — show next 5 fire times. `mu cron history` — recent run history from system logs. Thin wrapper around system crontab with validation and preview. | ⭐ ~60 lines |
| 🥉 | **DB Query** (`mu db`) | Ad-hoc database queries without a GUI: `mu db mysql -h host -u user -p "SELECT * FROM users LIMIT 10"`, `mu db psql ...`, `mu db sqlite ./app.db ".schema"`. Schema inspection (`mu db schema mysql -h host users`), CSV/JSON export. Complements `proxy` (which is a long-running proxy server, not ad-hoc queries). | ⭐⭐⭐ ~200 lines |
| 🥉 | **SSH Utilities** (`mu ssh`) | `mu ssh hosts` — parse `~/.ssh/config` and list hosts with connection details. `mu ssh tunnel --local 8080 --remote 127.0.0.1:3000 --host prod`. `mu ssh known-hosts check github.com` — verify/refresh known_hosts entries. `mu ssh config gen` — generate SSH config blocks. | ⭐⭐ ~100 lines |
| — | **Health Check** (`mu health`) | `mu health check https://api.example.com/health --retries 3 --timeout 5s`. `mu health check-file targets.yaml` — batch check. Output in table or Prometheus exposition format. Notification hooks (webhook/email on failure). Pairs with `fleet` for checking remote service health. | ⭐⭐ ~80 lines |
| — | **Secret Manager** (`mu secret`) | Lightweight encrypted secrets: `mu secret set API_KEY`, `mu secret get API_KEY`, `mu secret list`, `mu secret rotate TOKEN`. Local encrypted storage (AES-256-GCM with key from `keyring`). Bridges to the keyring plan in [keyring-module-plan.md](./keyring-module-plan.md). | ⭐⭐ ~100 lines |

## Recently Completed

- **Log Tailer** — `mu log`: tail and filter log files (`internal/log/` + `internal/core/logtail/`). Supports multi-file tail, follow mode (`-f`), minimum level filter (`-l`), time window (`--since`), regex filter (`--grep`), line limit (`-n`), and auto-detection of JSON-line vs plain-text formats with colorized level highlighting (DEBUG=faint, INFO=green, WARN=yellow, ERROR=red, FATAL=red+bold). Follow mode polls every 500 ms and handles file truncation/rotation. See [docs/log.md](./log.md)
- **HTTP Client** — merged into `mu network http` (curl-like subcommand). Core logic moved to `internal/core/httpclient/`, CLI in `internal/network/httpclient.go`. Supports all methods (`-X`), repeatable request headers (`-H`), body via `-d` or stdin, Bearer auth (`-A`), timeout (`-t`), TLS skip (`-k`), no-redirect (`-N`), forced JSON pretty-print (`-j`), body-only mode (`-b`), and file output (`-o`). Auto-detects JSON responses, color-codes the status line (green 2xx / red 4xx+), and writes a one-line summary to stderr so the body pipes cleanly. See [docs/network.md](./network.md)
- **Rust SCIP indexer** — rust-analyzer (`scip` subcommand) registered in `internal/core/scip`: bare `.gz` single-binary extraction fallback (`extractRawGzip`, tried after tar parse), `Cargo.toml`/`*.rs` detection, data-driven invocation `rust-analyzer scip --output <path> .`; enables `find_references`/`find_definition`/`symbol_info`/`read_function` for Rust repos via `git review`
- **Frontend Favicons** — Added emoji favicons to all 11 frontends matching gateway landing page card icons
- **Mock Dynamic Server Fix** — Fixed gateway integration default route fallback to DynamicRouter for mock endpoints
- **svcreg Web Dashboard** — Svelte 5 frontend with Dashboard, Services, Instances, Admin tabs.
  Admin tab includes server lifecycle management (start/stop with port/host/DB path configuration),
  independent process group option for gateway restart safety, PID file recovery, and live logs.
  Frontend connects to svcreg serve via proxy API; CLI refactored with `Client` module.
- **svcreg Service Registry** — Lightweight Apache ServiceComb ServiceCenter-compatible
  server with BoltDB storage, REST API (25+ endpoints), WebSocket watcher, heartbeat
  lease management, environment-based isolation, batch instance query, request logging
  middleware, and CLI client commands (`status`, `list services`, `list instances`)
- **Mock Dynamic Server** — Configurable multi-endpoint mock with template engine, conditional responses, delay simulation, and verbose logging
- **Admin Web UI** — Svelte 5 frontend with CodeMirror 6 JSON editor, endpoint CRUD, and config persistence
- **Custom DynamicRouter** — Thread-safe runtime endpoint registry with path parameter matching, replaces static `http.ServeMux`
- **Gateway Integration** — Mock admin available at `/mock/` via `mu gateway`, auto-discovered from `~/.config/mu/mock-config.json`
- **Unified Dark/Light Theme** — All frontends (gateway, mock, WOL, ES) share CSS variables and localStorage-based theme toggle with `mu-theme` key
- **QR Code Web UI** — Svelte 5 frontend with text input, level selector, PNG generation via `/api/qrcode`, and gateway integration at `/qrcode/`
- **JAR Analyzer Web UI** — Svelte 5 frontend with file upload, detailed analysis display, `/api/jarinfo/analyze` API, and gateway integration at `/jarinfo/`
- **Crypto Web UI** — Svelte 5 frontend with password generator, AES/DES/3DES/SM4 encrypt/decrypt, clipboard fallback, and gateway integration at `/crypto/`
- **JWT Decode/Verify** — CLI and web UI for JWT token decoding and HMAC signature verification with auto-detected algorithm and base64 key support
- **Encode/Decode** — CLI and web tab for base64, base64url, hex, URL encode/decode
- **Password Options** — `--no-digits` and `--special` flags for password generator
- **Diff Web UI** — Full-page CodeMirror merge view with real-time diff, synchronized scrolling, file upload, localStorage persistence, and gateway integration at `/diff/`
- **k8s Secret Tool** — CLI tool to generate and decode Kubernetes Opaque Secret YAML from key=value pairs, env files, or stdin
- **k8s Web UI** — Svelte 5 frontend with Secret Generator, Decode Secret tabs, .env file loading, and gateway integration at `/k8s/`
- **k8s Resource Listing** — `mu k8s get pods|nodes|deployments|services|configmaps|namespaces|statefulsets|daemonsets|ingresses|secrets` using `k8s.io/client-go`, supports `--context` and `--kubeconfig`
- **k8s Web UI Resources** — Resources tab with kubeconfig upload/paste, multi-config management, context switching, namespace dropdown, 10 resource types listing
- **k8s Describe** — CLI and web UI describe command for 10 resource types with detailed resource info in a modal dialog
- **Misc Tools** — `mu misc uuid|json|timestamp|hash` with CLI and web UI (4 tabs), zero external dependencies, gateway integration at `/misc/`
- **Network Tools** — `mu network dns|dig|whois` with CLI and web UI (3 tabs), uses `github.com/miekg/dns` and `github.com/likexian/whois`, gateway integration at `/network/`
- **k8s Metrics** — `mu k8s get nodes --metrics` and `mu k8s get pods --metrics` with CPU/memory usage, percentage for nodes, optional in Web UI via "Show metrics" checkbox; zero new dependencies
- **Ask Command** — `mu ask` command for LLM Q&A with concise answers and reference URLs, using `internal/core/openai` client and `internal/core/llm` shared config
- **Web Search Integration** — `mu ask --search` flag that fetches Brave Search API results and injects them into the LLM prompt for up-to-date, cited answers
- **Shared LLM Config** — `internal/core/llm` package to deduplicate config loading/saving logic across LLM-using commands
- **git review agent** — Multi-turn tool-calling code review agent with 7 tools (read_file, read_diff, search_code, read_function, find_references, find_definition, symbol_info); agent loop via `internal/core/openai.ChatWithTools()`; output rendered via `glamour` + `less -R` pager; reviews saved to `~/.cache/mu/git_reviews/` with YAML front matter; `mu git review --list` for browsing
- **SCIP semantic code intelligence** — `internal/core/scip` package + `mu scip` command; treesitter-nvim-style on-demand indexer install (auto-download from GitHub release into `~/.cache/mu/scip/tools/`), commit-cached SCIP indexes (`~/.cache/mu/scip/index/`), and 4 semantic agent tools (`find_references`, `find_definition`, `symbol_info`, upgraded `read_function`); graceful degradation to text tools; `review.scip` config in `git-config.json`
- **git-config.json** — Unified config replacing `commit-config.json`: `providers` array with named LLM providers, `commit`/`review` module configs referencing providers; automatic migration from old config
- **Java SCIP indexer** — scip-java enabled via `internal/core/scip`: `AssetName` by-name download of the cross-platform JVM launcher (SHA256-verified via the companion `.sha256` asset), data-driven indexer args (`Prefix`/`OutputFlag`/`QuietFlag`/`Trailing`), unified temp-file output capture (`runIndexer` + `extractError`, retained log path on failure), visible build line + spinner, and `FailHard` (Java build failure aborts `git review`; Go falls back); `--target` guard against non-HEAD commits
- **Agent review-loop optimizations** — `read_file` results cached per path/range (re-reads return a short note, not content), identical tool calls within one step deduped, and an "already-read files" system hint injected as the read set grows; measured 19→16 rounds with deeper reviews (7 findings) and lower token usage
- **"no changes" error hints** — `git review` empty-diff errors now detect untracked files (suggest `git add -N`) and staged changes (suggest `--staged`/`git reset`), consistent with `git diff` semantics
- **AssetName download SHA256 verification** — `internal/core/installer.AssetByURL` now resolves the companion `.sha256` checksum (via `fetchChecksum`) so OS/arch-less launcher downloads are integrity-checked
- **Version package** — Version vars moved from `cmd/mu/version.go` to `internal/core/version`; Makefile ldflags target the new package
- **Standard layout migration** — Migrated to `cmd/ + internal/` Go layout; all business logic moved from CLI packages to `internal/core/`
- **Nix flake dev environment** — Added `flake.nix` + `flake.lock` for reproducible development environment