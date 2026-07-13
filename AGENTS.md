# AGENTS.md

This file provides guidance to coding agents when working with code in this repository.

## Build Commands

This is a Go project using a Makefile for builds:

```bash
# Build for current platform
go build -o bin/mu

# Build for specific platforms
make linux-amd64      # Linux x86_64
make linux-armv8      # Linux ARM64
make darwin-arm64     # macOS Apple Silicon
make windows-amd64    # Windows x86_64
make all              # Build all common platforms

# Clean build artifacts
make clean
```

Builds output to `bin/` directory with naming pattern `mu-<platform>`.

## Test Commands

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./core/watcher/

# Run a single test function
go test -run TestFunctionName ./core/watcher/
```

## Lint Commands

```bash
# Standard Go formatting and vetting
go fmt ./...
go vet ./...
```

## Project Architecture

This is a CLI tool named `mu` (myUtilities) built with the Kong CLI framework. The architecture follows a command-based structure:

### Entry Point

- `main.go` - Entry point using Kong for CLI parsing. Version info is injected at build time via ldflags.
- `myutilities.go` - Defines the root command structure with subcommands
- `version.go` - Version variables (populated by Makefile during build)

### Command Structure

Commands are organized in separate packages, each with an `options.go` defining flags and a `Run()` method:

```
myutilities.go
├── Installer (cmd: install)    - Install binaries from GitHub releases
├── Mocker (cmd: mock)          - Mock servers for testing
├── Proxy (cmd: proxy)          - Database proxy
├── Runner (cmd: run)           - Command runner with display
├── Wol (cmd: wol)              - Wake-on-LAN HTTP server with agent
├── Crypto (cmd: crypto)        - Crypto utilities
├── Gateway (cmd: gateway)      - Unified gateway server
├── Es (cmd: es)                - Elasticsearch query tool
└── Commit (cmd: commit)        - AI-generated conventional commit messages
```

### Design Convention

Command packages (`<cmd>/`) are **CLI wrappers only**. They handle:

- CLI flag definitions (Kong struct tags)
- Configuration loading
- User interaction (prompts, confirmations, colored output)
- Delegation to core packages

Business logic, API clients, and platform operations belong in `core/` packages, exposed as
public functions and structs so they can be reused across commands or tested independently.

Example — `commit/` → `core/openai/` + `core/git/`:

- `commit/command.go` — Options struct, Run(), interactive prompt, editor, systemPrompt
- `commit/config.go` — CLI-specific config (`CommitConfig`, `~/.config/mu/commit.json`)
- `core/openai/client.go` — `Client` struct, `ChatCompletion()` → `*ChatResult`
- `core/git/git.go` — `CheckPreflight()`, `GetStagedDiff()`, `GetStagedNameStatus()`

### Core Packages

The `core/` directory contains reusable business logic:

- `core/proxy/` - Proxy abstractions and database-specific implementations
  - `Proxy.go` - Base proxy interface and `DefaultProxy` struct
  - `db/DBProxy.go` - Oracle database proxy with health checks
- `core/runner/` - Command execution with real-time output display
  - `CommandRunner.go` - Runs bash commands with colored output and buffer management
- `core/watcher/` - Event-driven resource watching system
  - Implements a Kubernetes-style watch pattern with `WatchServer`, `EventStore`
  - `Watcher` interface for pluggable resource monitors
- `core/net/` - Network utilities
  - `SendWOL()` - Wake-on-LAN magic packet sender
  - `GetInterfaceDetails()`, `SelectBestInterfaceForWOL()` - Network interface discovery
  - `GetOutboundMAC()` - Resolves the MAC address of the interface used to reach a given server via UDP route lookup
  - `ValidHostname()`, `ValidMAC()` - Input validation
- `core/store/` - BoltDB key-value store
  - `Store` struct with mutex-guarded BoltDB operations
  - Buckets: `Aliases` (hostname→MAC), `Boot` (boot timestamps), `Status` (boot/shutdown state)
- `core/openai/` - OpenAI-compatible chat completions API client
  - `Client` struct with `ChatCompletion(systemPrompt, userPrompt)` → `*ChatResult`
  - `ChatResult` includes content, prompt tokens, completion tokens, total tokens
- `core/git/` - Git operations
  - `CheckPreflight()` — verifies git is installed and in a repository
  - `GetStagedDiff()` — returns staged diff + stat with truncation
  - `GetStagedNameStatus()` — returns `git diff --staged --name-status`

### Command Packages

- `installer/` - GitHub release installer
  - Fetches releases from GitHub API
  - Generates shell install scripts via templates
  - Supports asset selection by OS/arch
  - `templates/install.sh.tmpl` - Shell script template

- `mock/` - Mock servers for development/testing
  - `mock-server` - HTTP mock server with CSV data or random generated data
  - `file-server` - File upload server with multipart form support
  - `oauth-server` - OAuth2 mock server (delegates to `oauth/` package)

- `proxy/` - Database proxy command
  - Currently supports Oracle database proxy with failover
  - Health checks via TCP and SQL queries

- `runner/` - Command runner
  - Executes bash commands sequentially
  - Displays real-time output with ANSI colors

- `wol/` - Wake-on-LAN HTTP server and agent
  - `serve` subcommand: HTTP server with Svelte frontend, alias CRUD, WOL magic packet sending
  - `agent` subcommand: sends boot/shutdown/register notifications to the WOL server with retry backoff
  - `interfaces` subcommand: lists available network interfaces with WOL suitability info
  - Embeds compiled Svelte frontend via `//go:embed` (requires `npm run build` in `wol/frontend/`)

- `commit/` - AI-generated conventional commit messages
  - Uses `core/openai` for ChatCompletion API calls
  - Uses `core/git` for diff gathering and preflight checks
  - Supports `--diff-strategy` (auto/full/summary) for different diff sizes
  - Interactive confirmation with editor-based edit support
  - Colorized output via `github.com/morikuni/aec`

## Web Frontend Architecture

Modules with a web UI (served through the gateway or standalone) must follow these conventions.

### File Structure

```
<module>/
├── embed.go                #go:embed frontend/dist + FrontendHandler()
├── frontend/
│   ├── package.json        svelte + vite
│   ├── vite.config.js      base: './'
│   ├── index.html          <!-- inject:theme --> + <!-- inject:common --> placeholders
│   └── src/
│       ├── main.js         mount Svelte app
│       └── App.svelte      main component
```

### Shared Partials

All frontends inject shared CSS/JS via placeholders at build time (handled by `make frontend`):

| File | Placeholder | Contents |
|---|---|---|
| `shared/frontend/theme-partial.html` | `<!-- inject:theme -->` | CSS variables (`--bg`, `--surface`, `--primary`, etc.), theme toggle JS, `.toggle-btn` styles |
| `shared/frontend/common-partial.html` | `<!-- inject:common -->` | Shared class styles (`.home-link`, `.btn`, `.card`, `.msg`) |

**Rules:**
- Every `index.html` MUST contain both `<!-- inject:theme -->` and `<!-- inject:common -->` placeholders.
- CSS variables in theme-partial are the single source of truth for all colors.
- Modifying shared styles in these partials affects all 6+ frontends simultaneously.
- Per-module style overrides belong in `App.svelte`'s `<style>` block (Svelte-scoped).

### Required Go Exports

Each module's Go package must export:

```go
// FrontendHandler serves the embedded Svelte frontend (SPA with index.html fallback).
func FrontendHandler() http.Handler

// RegisterHandlers registers all API routes on the given mux.
// Routes are typically under /api/<module>/... (root-level, not prefixed).
func RegisterHandlers(mux *http.ServeMux)
```

`embed.go` follows this pattern (same as `wol/embed.go`, `mock/dynamic_admin.go`, etc.):

```go
//go:embed frontend/dist
var frontendFS embed.FS

func FrontendHandler() http.Handler { /* ... */ }
```

### Serve Subcommand

Every web-enabled module MUST provide a standalone serve subcommand:

```go
type ServeOptions struct {
    Port int `help:"Port to listen on." default:"808x"`
}

func (o *ServeOptions) Run() error {
    mux := http.NewServeMux()
    mux.Handle("/", FrontendHandler())
    RegisterHandlers(mux)
    return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}
```

This allows running independently: `mu <module> serve --port <port>`.

### Required Frontend Features

Every `App.svelte` MUST include:

- **Theme toggle** — Uses `window.__MU_GATEWAY__` detection (server-side injected by gateway) and shared `toggleTheme()` JS for dark/light switching.
- **`← Home` button** — Conditionally shown via `{#if inGateway}` (reads `window.__MU_GATEWAY__`).
- **CSS variable theming** — All colors use `var(--bg)`, `var(--surface)`, `var(--text)`, etc. from the shared partials.
- **Copy button with fallback** — Uses `navigator.clipboard.writeText` with `execCommand('copy')` fallback for non-HTTPS environments.

### Gateway Integration

In `gateway/command.go`, register the module following this pattern:

```go
import "github.com/yusiwen/myUtilities/<module>"

// In Run():
mux.Handle("/<module>/", http.StripPrefix("/<module>", withGateway(<module>.FrontendHandler())))
<module>.RegisterHandlers(mux)
```

Also add a landing page card in the `landingPage()` function and a log entry listing the route.

### Registration Checklist

When adding a new module with a web UI:

1. `Makefile` — Add `<MODULE>_FRONTEND_DIR` variable and build step in `frontend` target.
2. `.gitignore` — Add `frontend/node_modules/` and `frontend/dist/`.
3. `gateway/command.go` — Import package, register frontend + API, add landing card, add log entry.
4. `README.md` — Document CLI usage + `serve` subcommand + gateway route.
5. `shared/frontend/` partials — No changes needed unless new CSS variables or shared classes are required.

### Key Dependencies

- `github.com/alecthomas/kong` - CLI framework
- `github.com/sijms/go-ora/v2` - Oracle database driver
- `github.com/go-git/go-git/v5` - Git operations
- `github.com/ryanolee/go-chaff` - Mock data generation
- `github.com/morikuni/aec` - ANSI escape codes for terminal colors

### Build-time Variables

The Makefile injects version info at build time:
- `main.Version` - Git tag or "unknown version"
- `main.CommitSHA` - Short git commit hash
- `main.BuildTime` - Build timestamp (UTC)

### Testing

Minimal test coverage currently exists. Only `core/watcher/watcher_test.go` contains tests.

## Release Process

When making a new release:
1. Get the latest git tag starts with 'v' as current version
2. Increase current version following semantic version rules (e.g. `v1.0.8`), and ask user to confirm
3. Update `VERSION` in `install.sh` to the new tag (e.g. `v1.0.8`)
4. Update any version references in `README.md`
5. Commit with message with new version, e.g. `chore: bump version to v1.0.8`
6. Tag the commit with new version, e.g. `git tag -a v1.0.8 -m "v1.0.8"` (use `-a` + `-m` to avoid triggering interactive editor for tag message)
7. Push: `git push && git push --tags`
8. Let the CI/CD workflow create the GitHub release with built assets

### Notes

- The project uses Go 1.24 (see `go.mod`)
- Cross-compilation is supported for multiple platforms (Linux, macOS, Windows, FreeBSD, MIPS)
- The `docs/tasks.md` file contains a backlog of improvement tasks
