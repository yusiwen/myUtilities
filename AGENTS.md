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
└── Wol (cmd: wol)              - Wake-on-LAN HTTP server with agent
```

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
  - `ValidHostname()`, `ValidMAC()` - Input validation
- `core/store/` - BoltDB key-value store
  - `Store` struct with mutex-guarded BoltDB operations
  - Buckets: `Aliases` (hostname→MAC), `Boot` (boot timestamps), `Status` (boot/shutdown state)

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
  - `agent` subcommand: sends boot/shutdown notifications to the WOL server with retry backoff
  - `interfaces` subcommand: lists available network interfaces with WOL suitability info
  - Embeds compiled Svelte frontend via `//go:embed` (requires `npm run build` in `wol/frontend/`)

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

### Notes

- The project uses Go 1.24 (see `go.mod`)
- Cross-compilation is supported for multiple platforms (Linux, macOS, Windows, FreeBSD, MIPS)
- The `docs/tasks.md` file contains a backlog of improvement tasks
