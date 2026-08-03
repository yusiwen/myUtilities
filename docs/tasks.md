# myUtilities Improvement Tasks

This document contains a list of actionable improvement tasks for the myUtilities project. Each task is marked with a checkbox that can be checked off when completed.

## Code Organization and Structure

> 详细执行方案见 [Codebase Restructure Plan](./codebase-restructure-plan.md)（Phases 1-4）

1. [x] Refactor the project to follow standard Go project layout (cmd/, pkg/, internal/, etc.)
    → See [Phase 3 — Standard Layout](./codebase-restructure-plan.md#phase-3--标准项目布局)
    → See [cmd-internal-restructure-plan.md](./cmd-internal-restructure-plan.md)（执行方案与进度）
2. [ ] Move version information to a dedicated package for better maintainability
    → See [Phase 1-①](./codebase-restructure-plan.md#1-①-版本信息抽到独立包)
3. [ ] Create separate packages for common utilities instead of embedding them in specific packages
    → See [Phase 2-④](./codebase-restructure-plan.md#2-④-解决-4-组包名冲突)
4. [ ] Standardize naming conventions across the codebase
    → See [Phase 1-②](./codebase-restructure-plan.md#1-②-修复-pascalcase-文件名) + [Phase 4-⑥](./codebase-restructure-plan.md#4-⑥-统一命名规范)
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

## Build and Deployment

36. [ ] Update the Makefile to support all target platforms
37. [x] Implement semantic versioning
38. [ ] Automate the release process completely
39. [ ] Add containerization support (Docker)
40. [ ] Create installation packages for different package managers (apt, brew, etc.)

## User Experience

41. [x] Improve command-line help messages and documentation
42. [ ] Add color and formatting to terminal output
43. [ ] Implement interactive mode for complex operations
44. [x] Add command completion for shells
45. [x] Create a web UI for the mock services

## Web UI Candidates (New Feature Ideas)

Simple tools that would benefit from a web UI and gateway integration:

| Priority | Module | Description | Backend Effort | Frontend Effort |
|---|---|---|---|---|
| 🥇 | **JSON Tool** | Format, validate, compress, and query JSON (reuse CodeMirror) | ⭐ ~10 lines | ⭐ ~100 lines |
| 🥇 | **UUID** | Generate UUID v1/v4/v7, single or batch | ⭐ ~20 lines | ⭐ ~50 lines |
| 🥈 | **Timestamp** | Unix timestamp ↔ human date/time, auto-detect format | ⭐ ~30 lines | ⭐ ~60 lines |
| 🥈 | **Hash** | File upload or text input → SHA1/SHA256/SHA512/MD5 | ⭐⭐ ~40 lines | ⭐ ~80 lines |
| 🥉 | **Port Scan** | TCP port scanning from server | ⭐⭐ ~60 lines | ⭐ ~80 lines |
| 🥉 | **DNS Lookup** | DNS record queries (A/AAAA/MX/NS/TXT) | ⭐ ~40 lines | ⭐ ~60 lines |
| — | **HTTP Client** | Web-based curl: method, URL, headers, body → response | ⭐⭐⭐ ~80 lines | ⭐⭐⭐ ~150 lines |
| — | **watch** dashboard | File/git watch events via SSE stream to browser | ⭐⭐ (core/watcher ready) | ⭐⭐⭐ ~200 lines |
| — | **git commit** UI | Stage files, view diff, generate/edit commit message via LLM | ⭐⭐ (core/git + openai ready) | ⭐⭐⭐ ~200 lines |

## Recently Completed

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

## Recently Completed

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
- **Ask Command** — `mu ask` command for LLM Q&A with concise answers and reference URLs, using `core/openai` client and `core/llm` shared config
- **Web Search Integration** — `mu ask --search` flag that fetches Brave Search API results and injects them into the LLM prompt for up-to-date, cited answers
- **Shared LLM Config** — `core/llm` package to deduplicate config loading/saving logic across LLM-using commands
- **git review agent** — Multi-turn tool-calling code review agent with 7 tools (read_file, read_diff, search_code, read_function, find_references, find_definition, symbol_info); agent loop via `core/openai.ChatWithTools()`; output rendered via `glamour` + `less -R` pager; reviews saved to `~/.cache/mu/git_reviews/` with YAML front matter; `mu git review --list` for browsing
- **SCIP semantic code intelligence** — `core/scip` package + `mu scip` command; treesitter-nvim-style on-demand indexer install (auto-download from GitHub release into `~/.cache/mu/scip/tools/`), commit-cached SCIP indexes (`~/.cache/mu/scip/index/`), and 4 semantic agent tools (`find_references`, `find_definition`, `symbol_info`, upgraded `read_function`); graceful degradation to text tools; `review.scip` config in `git-config.json`
- **git-config.json** — Unified config replacing `commit-config.json`: `providers` array with named LLM providers, `commit`/`review` module configs referencing providers; automatic migration from old config
- **Java SCIP indexer** — scip-java enabled via `core/scip`: `AssetName` by-name download of the cross-platform JVM launcher (SHA256-verified via the companion `.sha256` asset), data-driven indexer args (`Prefix`/`OutputFlag`/`QuietFlag`/`Trailing`), unified temp-file output capture (`runIndexer` + `extractError`, retained log path on failure), visible build line + spinner, and `FailHard` (Java build failure aborts `git review`; Go falls back); `--target` guard against non-HEAD commits
- **Agent review-loop optimizations** — `read_file` results cached per path/range (re-reads return a short note, not content), identical tool calls within one step deduped, and an "already-read files" system hint injected as the read set grows; measured 19→16 rounds with deeper reviews (7 findings) and lower token usage
- **"no changes" error hints** — `git review` empty-diff errors now detect untracked files (suggest `git add -N`) and staged changes (suggest `--staged`/`git reset`), consistent with `git diff` semantics
- **AssetName download SHA256 verification** — `core/installer.AssetByURL` now resolves the companion `.sha256` checksum (via `fetchChecksum`) so OS/arch-less launcher downloads are integrity-checked