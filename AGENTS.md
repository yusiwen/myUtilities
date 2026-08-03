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
├── Scip (cmd: scip)            - SCIP semantic code intelligence (indexers + index)
├── Gateway (cmd: gateway)      - Unified gateway server
├── Es (cmd: es)                - Elasticsearch query tool
├── Git (cmd: git)              - Git utilities (ignore, commit, review)
└── ...
```

### Design Convention

Command packages (`<cmd>/`) are **CLI wrappers only**. They handle:

- CLI flag definitions (Kong struct tags)
- Configuration loading
- User interaction (prompts, confirmations, colored output)
- Delegation to core packages

Business logic, API clients, and platform operations belong in `core/` packages, exposed as
public functions and structs so they can be reused across commands or tested independently.

Example — `git/` → `core/openai/` + `core/git/`:

- `git/commit.go` — Options struct, Run(), interactive prompt, editor, systemPrompt
- `core/git/agent.go` — Multi-turn agent loop with `ChatWithTools`, 7 tools (read_file, read_diff, search_code, read_function, find_references, find_definition, symbol_info)
- `git/config.go` — `GitConfig` with `providers` array, `commit`/`review` module configs; `git-config.json`
- `core/openai/client.go` — `Client` struct, `ChatCompletion()` → `*ChatResult`, `ChatWithTools()` → `*ChatResponse`
- `core/git/git.go` — `CheckPreflight()`, `GetStagedDiff()`, `GetDiff()`, `GetNameStatus()`, `GetUntrackedFiles()`, `ResolveRev()`, `IsDirty()`; `noChangesErr()` hints at untracked/staged files on an empty diff

### Configuration File Convention

All module configuration files follow the pattern `<module>-config.json`
stored under `~/.config/mu/` by default:

| Module | Config File |
|--------|-------------|
| ask | `ask-config.json` |
| budget | `budget-config.json` |
| git | `git-config.json` |
| es | `es-config.json` |
| mock | `mock-config.json` |
| svcreg | `svcreg-config.json` |
| wol | `wol-config.json` |

**Directory override:** The `mu gateway --config-dir <path>` flag
overrides the base directory for all gateway-integrated modules.
Modules with config files should accept a config path parameter in their
`LoadConfig` or `RegisterHandlers` functions; when the path is empty,
fall back to `~/.config/mu/<module>-config.json`.

**Security:** Config files containing secrets must use file permission
`0600`. Future versions will support OS keyring storage
(`mu secret set/get`) as a more secure alternative.

**Do NOT hardcode** `~/.config/mu` in module code — always accept a
config path parameter and fall back to the default only when empty.

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
  - `ChatWithTools(messages, tools)` → `*ChatResponse` with `ToolCalls` for agent loops
  - `ChatResult` includes content, prompt tokens, completion tokens, total tokens
  - `ChatResponse` extends `ChatResult` with `ToolCalls []ToolCall`
  - Types: `Message`, `ToolDef`, `ToolFunction`, `ToolCall`, `ToolCallFunc`
- `core/git/` - Git operations
  - `CheckPreflight()` — verifies git is installed and in a repository
  - `GetStagedDiff()` — returns staged diff + stat with truncation
  - `GetStagedNameStatus()` — returns `git diff --staged --name-status`
  - `GetDiff(args)` — generic diff with arbitrary args (staged, branch comparison, paths)
  - `GetNameStatus(args)` — generic `--name-status` for arbitrary diff args
  - `GetUntrackedFiles()` — returns list of untracked files
  - `RepoName()`, `CurrentBranch()`, `ShortCommit()` — repo metadata helpers
- `core/scip/` - SCIP semantic code intelligence
  - `EnsureIndex(opts)` — detects repo languages, auto-downloads indexer binaries (reusing `core/installer`), generates commit-cached SCIP indexes, returns a loaded `IndexSet`
  - `IndexSet` query API: `FindDefinition(path, line)`, `FindReferences(path, line)`, `SymbolsInRange`, `SymbolInfoAt`, `IndexFor(path)`
  - `Indexer` registry — per-language indexer metadata (detect signals, GitHub release, pinned version, `AssetName` for OS/arch-less launchers, data-driven `Prefix`/`OutputFlag`/`QuietFlag`/`Trailing` args, `FailHard`); Go and Java are enabled (Java is `FailHard`), TS/C registered for future use
  - `EnsureOptions` — `RepoRoot`, `CacheDir` (default `~/.cache/mu/scip`), `AutoInstall`, `Force`
  - Cache layout: `~/.cache/mu/scip/tools/<name>/<version>/<binary>` and `~/.cache/mu/scip/index/<project>/<lang>/<commit>.scip` (or `working.scip` when dirty)
  - Dirty-tree freshness: `working.scip` is reused only if no matching changed source file (via `git status --porcelain`) is newer than it; `--refresh-scip` forces a rebuild. Clean trees always reuse the immutable per-commit index.
  - Indexer output is captured to a temp file (`runIndexer`); on failure `extractError` shows relevant lines and the full log is retained with its path. Stale/forced rebuilds print a visible reason line + spinner (`newIndexSpinner`, TTY only).
  - `AssetName` downloads (OS/arch-less launchers like scip-java) are SHA256-verified via the companion `.sha256` release asset; `core/installer.AssetByURL` resolves URL + checksum, `fetchChecksum` parses it
  - Build failures return an `IndexError{Lang, Err, Hard}`; `FailHard` languages (Java) abort `git review` via `git/review.go`, while Go falls back to text tools.
  - Consumed by `core/git/agent.go` (tools: `find_references`, `find_definition`, `symbol_info`, upgraded `read_function`)

### Command Packages

- `installer/` - GitHub release installer
  - Fetches releases from GitHub API
  - Generates shell install scripts via templates
  - Supports asset selection by OS/arch
  - `templates/install.sh.tmpl` - Shell script template
  - `core/installer` also exposes `AssetByURL` (resolve an asset by exact name + its SHA256 companion checksum, used by `core/scip` for OS/arch-less launchers)

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

- `git/` - Git utilities (ignore, commit, review)
  - `ignore` — Downloads .gitignore templates from GitHub
  - `commit` — AI-generated conventional commit messages using `core/openai` + `core/git`
  - `review` — Multi-turn AI code review agent:
    - 7 tools: `read_file`, `read_diff`, `search_code`, `read_function`, `find_references`, `find_definition`, `symbol_info`
    - Agent loop with `core/openai.ChatWithTools()`; tool results are `read_file`-cached per path/range (re-reads return a note, not content), identical calls within one step are deduped, and an "already-read files" system hint is injected as the read set grows
    - Output rendered via `glamour` + `less -R` pager
    - Reviews saved to `~/.cache/mu/git_reviews/<project>_<branch>_<timestamp>.md`
    - Shared config with `commit` via `git-config.json`
    - Optional SCIP semantic tools via `core/scip` (upgraded `read_function`); controlled by `--no-scip`/`--refresh-scip` and `review.scip` config
    - Empty-diff errors hint at untracked (`git add -N`) and staged (`--staged`/`git reset`) files, consistent with `git diff` semantics

- `scip/` - SCIP semantic code intelligence command
  - `install <lang>` — treesitter-nvim-style auto-download of an indexer binary
  - `list` — available/installed indexers
  - `index` — build the index for the current repo
  - `purge` — remove cached indexers and indexes

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
- `github.com/charmbracelet/glamour` - Terminal markdown rendering

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
