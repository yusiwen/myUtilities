# CODEBASE.md

## Overview

`mu` (myUtilities) is a Go CLI tool built with the Kong CLI framework. It bundles multiple utility commands: GitHub release installer, mock servers, database proxy, command runner, Wake-on-LAN server, and Elasticsearch query UI.

- **Module:** `github.com/yusiwen/myUtilities`
- **Go:** 1.24
- **CLI:** `github.com/alecthomas/kong` v1.12.1

---

## Entry Points

| File | Purpose |
|---|---|
| `main.go:14` | `main()` — parses CLI with Kong, dispatches to subcommands |
| `myutilities.go:13` | `MyUtilities` struct — defines top-level subcommands |
| `version.go:3` | Build-time injected vars: `Version`, `CommitSHA`, `BuildTime` |

**Commands registered** (`myutilities.go`):
`install`, `mock`, `proxy`, `run`, `wol`, `es`

---

## Build System (Makefile)

- Build output: `bin/mu-<platform>`
- Version/commit/time injected via `-ldflags`
- Frontend: builds Svelte apps under `wol/frontend` and `es/frontend` via `npm run build`, embeds with `//go:embed`
- Supported platforms: darwin (amd64/arm64), linux (386/amd64/armv5-v8/mips*), freebsd (386/amd64/arm64), windows (386/amd64/arm64/arm32v7)

---

## Project Layout

```
.
├── main.go                  # Entry point
├── myutilities.go           # CLI command definitions
├── version.go               # Build-time version vars
├── Makefile                 # Cross-compilation builds
├── core/                    # Shared business logic
│   ├── net/                 #  Network utilities
│   │   ├── wol.go           #  SendWOL() — magic packet sender
│   │   ├── interface.go     #  IPFromInterface(), SelectBestInterfaceForWOL(), GetOutboundMAC()
│   │   ├── interfaces.go    #  GetInterfaceDetails(), type detection, WOL suitability
│   │   └── validation.go    #  ValidHostname(), ValidMAC()
│   ├── proxy/               # Database proxy abstractions
│   │   ├── Proxy.go         #  Proxy interface, BackendConfig, BackendStatus, DefaultProxy
│   │   └── db/DBProxy.go    #  OracleProxy — TCP proxy with health checks & failover
│   ├── runner/              # Command execution engine
│   │   └── CommandRunner.go #  Runs bash commands with real-time colored output, buffer mgmt
│   ├── store/               # BoltDB key-value store
│   │   └── store.go         #  CRUD for MAC aliases, boot/shutdown event recording
│   └── watcher/             # K8s-style watch system
│       ├── watcher.go       #  WatchServer, Watcher interface, event dispatch
│       ├── event.go         #  Event types, EventStore
│       ├── FileWatcher.go   #  Polls local files for changes (MD5 checksum)
│       └── GitWatcher.go    #  Polls remote Git repo for new commits, pulls changes
├── installer/               # GitHub release installer
│   ├── options.go           #  Flags: repo, output, token, os/arch override
│   ├── command.go           #  Run() — fetches releases, generates shell install scripts
│   ├── search.go            #  imFeelingLuck() — auto-discovers repo via DuckDuckGo/Google
│   ├── strings.go           #  Regex helpers: getOS, getArch, getFileExt
│   └── templates/
│       ├── templates.go     #  Embeds install.sh.tmpl
│       └── install.sh.tmpl  #  Shell script template for curl/untar install
├── mock/                    # Mock servers for testing
│   ├── options.go           #  Subcommands: file-server, mock-server, oauth-server
│   ├── fileserver.go        #  File upload server (multipart form)
│   ├── mockserver.go        #  HTTP mock with CSV or random generated data (chaff)
│   ├── oauthserver.go       #  Delegates to mock/oauth/ package
│   └── response.go          #  Response/Status structs
├── proxy/                   # Database proxy CLI
│   ├── options.go           #  Flags: host/port, db routes, health-check params
│   └── dbproxy.go           #  Run() — parses options, starts OracleProxy
├── runner/                  # Command runner CLI
│   ├── options.go           #  Embed: []Command from core/runner
│   └── runner.go            #  Run() — creates CommandRunner, executes commands
├── wol/                     # Wake-on-LAN HTTP server + agent
│   ├── options.go           #  Subcommands: serve, agent, interfaces
│   ├── command.go           #  Serve: WOL API, alias CRUD, boot/shutdown notify
│   │                       #  Agent: boot/shutdown/register with retry backoff
│   │                       #  Interfaces: list network interfaces with WOL suitability
│   └── embed.go             #  Embeds frontend/dist/* Svelte app
├── es/                      # Elasticsearch query tool
│   ├── options.go           #  Subcommands: set (host/user/password), serve
│   ├── command.go           #  Serve: HTTP server with /api/status, /api/indices, /api/search, /api/config
│   ├── client.go            #  go-elasticsearch client: newESClient, esPing, esListIndices, esSearch
│   ├── config.go            #  ESConfig, load/save JSON config, maskedPassword
│   └── embed.go             #  Embeds frontend/dist/* Svelte app
├── install.sh               # Quick install script for the tool itself
├── go.mod / go.sum
├── renovate.json
├── AGENTS.md                # Agent guidance for this project
├── README.md
├── .github/
├── .gitattributes
└── .gitignore
```

---

## Dependency Map

| Package | Purpose |
|---|---|
| `kong` | CLI framework |
| `go-ora/v2` | Oracle database driver (health checks) |
| `go-git/v5` | Git operations (GitWatcher) |
| `go-chaff` | Random mock data generation |
| `go-wol` | WOL magic packet marshaling |
| `go-elasticsearch/v8` | ES client |
| `golang-jwt/jwt/v5` | JWT for OAuth mock server |
| `bbolt` | Embedded key-value store |
| `aec` | ANSI escape codes for terminal colors |

---

## Key API Routes

### WOL Server (`wol/command.go`)
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/wake/{hostname}` | Send WOL magic packet |
| POST | `/api/register` | Agent registration |
| GET/POST | `/api/aliases` | List / create MAC aliases |
| DELETE | `/api/aliases/{name}` | Delete alias |
| GET/POST | `/api/notify/{hostname}` | Boot/shutdown events |

### ES UI (`es/command.go`)
| Method | Path | Purpose |
|---|---|---|
| GET | `/api/status` | ES connection status |
| GET | `/api/indices` | List ES indices |
| POST | `/api/search` | Execute ES query |
| GET/PUT | `/api/config` | View/update ES connection config |

### Mock Server (`mock/mockserver.go`)
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/mock/query/{rs}` | Paginated mock data query |

### File Server (`mock/fileserver.go`)
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/mock/file` | File upload |

---

## License

MIT
