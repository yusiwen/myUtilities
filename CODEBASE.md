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
| `cmd/mu/main.go:16` | `main()` — parses CLI with Kong, dispatches to subcommands |
| `cmd/mu/myutilities.go:13` | `MyUtilities` struct — defines top-level subcommands |
| `internal/core/version/version.go` | Build-time injected vars: `Version`, `CommitSHA`, `BuildTime` |

**Commands registered** (`cmd/mu/myutilities.go`):
`install`, `mock`, `qrcode`, `serve`, `svcreg`, `proxy`, `run`, `wol`, `es`,
`git`, `watch`, `k8s`, `jar`, `gateway`, `diff`, `network` (dns/dig/whois/http/serve), `misc`, `crypto`,
`ask`, `budget`, `metrics`, `scip`, `log`, `completion`

---

## Build System (Makefile)

- Build output: `bin/mu-<platform>`
- Version/commit/time injected via `-ldflags`
- Frontend: builds Svelte apps under `internal/wol/frontend` and `internal/es/frontend` via `npm run build`, embeds with `//go:embed`
- Supported platforms: darwin (amd64/arm64), linux (386/amd64/armv5-v8/mips*), freebsd (386/amd64/arm64), windows (386/amd64/arm64/arm32v7)

---

## Project Layout

```
.
├── cmd/
│   └── mu/
│       ├── main.go            # main() — parses CLI with Kong, dispatches to subcommands
│       └── myutilities.go     # MyUtilities struct — defines top-level subcommands
├── internal/                  # All application packages (not externally importable)
│   ├── core/                  # Shared business logic
│   │   ├── version/           # Build-time version vars (injected via ldflags)
│   │   ├── net/               #  Network utilities
│   │   │   ├── wol.go         #  SendWOL() — magic packet sender
│   │   │   ├── interface.go   #  IPFromInterface(), SelectBestInterfaceForWOL(), GetOutboundMAC()
│   │   │   ├── interfaces.go  #  GetInterfaceDetails(), type detection, WOL suitability
│   │   │   └── validation.go  #  ValidHostname(), ValidMAC()
│   │   ├── proxy/             # Database proxy abstractions
│   │   │   ├── proxy.go       #  Proxy interface, BackendConfig, BackendStatus, DefaultProxy
│   │   │   └── db/dbproxy.go  #  OracleProxy — TCP proxy with health checks & failover
│   │   ├── runner/            # Command execution engine (CommandRunner.go + recipe.go + recipe_runner.go)
│   │   │   └── CommandRunner.go #  bash -c execution with PTY/VT100 live display, spinner+elapsed header, Ctrl-C handling; recipe.go/recipe_runner.go add YAML task orchestration
│   │   ├── fleet/             # Remote batch execution (dispatcher + agent, poll model)
│   │   │   ├── dispatcher.go  #  HTTP API: jobs, register, poll, output, complete, files; token auth
│   │   │   ├── store.go       #  BoltDB buckets agents/jobs/runs/run_output, ClaimNextRun, 1MB output cap
│   │   │   ├── agent.go       #  Register → poll → download/extract files → run (core/runner) → stream → complete
│   │   │   ├── client.go      #  Controller/agent HTTP client
│   │   │   └── transfer.go    #  ArchiveExt / ExtractArchive (tar.gz/tar/zip) / ComputeSHA256
│   │   ├── httpclient/        # HTTP client core (curl alternative)
│   │   │   └── client.go      #  Do(Params) → *Result; Render(p, r); SummaryLine; ReadBodyFromStdin; PrettyJSON
│   │   ├── misc/              # Misc tools business logic
│   │   │   ├── uuid.go        #  GenUUID() — random v4 UUID
│   │   │   ├── json.go        #  FormatJSON / ValidateJSON / MinifyJSON
│   │   │   ├── timestamp.go   #  ConvertTimestamp() — unix/date parsing
│   │   │   ├── hash.go        #  Hash(alg, input) — md5/sha256/sha512
│   │   │   └── trackers.go    #  TrackersCache — TTL-cached tracker list fetch
│   │   ├── store/             # BoltDB key-value store
│   │   │   └── store.go       #  CRUD for MAC aliases, boot/shutdown event recording
│   │   └── watcher/           # K8s-style watch system
│   │       ├── watcher.go     #  WatchServer, Watcher interface, event dispatch
│   │       ├── event.go       #  Event types, EventStore
│   │       ├── filewatcher.go #  Polls local files for changes (MD5 checksum)
│   │       └── gitwatcher.go  #  Polls remote Git repo for new commits, pulls changes
│   ├── installer/             # GitHub release installer
│   │   ├── options.go         #  Flags: repo, output, token, os/arch override
│   │   ├── command.go         #  Run() — fetches releases, generates shell install scripts
│   │   ├── search.go          #  imFeelingLuck() — auto-discovers repo via DuckDuckGo/Google
│   │   ├── strings.go         #  Regex helpers: getOS, getArch, getFileExt
│   │   └── templates/
│   │       ├── templates.go   #  Embeds install.sh.tmpl
│   │       └── install.sh.tmpl  #  Shell script template for curl/untar install
│   ├── mock/                  # Mock servers for testing
│   │   ├── options.go         #  Subcommands: file-server, mock-server, oauth-server
│   │   ├── fileserver.go      #  File upload server (multipart form)
│   │   ├── mockserver.go      #  HTTP mock with CSV or random generated data (chaff)
│   │   ├── oauthserver.go     #  Delegates to mock/oauth/ package
│   │   └── response.go        #  Response/Status structs
│   ├── proxy/                 # Database proxy CLI
│   │   ├── options.go         #  Flags: host/port, db routes, health-check params
│   │   └── dbproxy.go         #  Run() — parses options, starts OracleProxy
│   ├── runner/                # Command runner CLI
│   │   ├── options.go         #  Flags: --command, --file (recipe), --task, --var, --keep-going, --dry-run, --schema
│   │   ├── runner.go          #  Run() — dispatches inline commands or recipe files (exit 130 on interrupt)
│   │   └── recipe_schema.go   #  Embeds docs/schema/recipe-schema.json for `mu run --schema`
│   ├── fleet/                 # Fleet CLI (serve/agent/run/hosts/status/jobs)
│   │   ├── options.go         #  Subcommands + flags (--hosts, --file/--command, --var, --files, --watch)
│   │   ├── command.go         #  Run() — wires to core/fleet; --watch streams output + per-host summary
│   │   └── config.go          #  fleet-config.json load (server/token/hostname/groups/poll/port/db/data_dir)
│   ├── wol/                   # Wake-on-LAN HTTP server + agent
│   │   ├── options.go         #  Subcommands: serve, agent, interfaces
│   │   ├── command.go         #  Serve: WOL API, alias CRUD, boot/shutdown notify
│   │   │                      #  Agent: boot/shutdown/register with retry backoff
│   │   │                      #  Interfaces: list network interfaces with WOL suitability
│   │   └── embed.go           #  Embeds frontend/dist/* Svelte app
│   ├── es/                    # Elasticsearch query tool
│   │   ├── options.go         #  Subcommands: set (host/user/password), serve
│   │   ├── command.go         #  Serve: HTTP server with /api/status, /api/indices, /api/search, /api/config
│   │   ├── client.go          #  go-elasticsearch client: newESClient, esPing, esListIndices, esSearch
│   │   ├── config.go          #  ESConfig, load/save JSON config, maskedPassword
│   │   └── embed.go           #  Embeds frontend/dist/* Svelte app
│   ├── ask/  budget/  completion/  crypto/  diff/  gateway/  git/  jarinfo/
│   ├── k8s/  log/  metrics/  misc/  qrcode/  scip/  serve/  svcreg/  watch/
│   ├── network/               # mu network — DNS, DIG, WHOIS, HTTP client (curl alternative)
│   │   ├── options.go         #  Subcommands: dns, dig, whois, http, serve
│   │   ├── command.go         #  DNS/DIG/WHOIS/Cert commands + HTTPClientOptions (mu network http)
│   │   └── embed.go           #  Embeds frontend/dist/* Svelte app
│   └── log/                   # mu log — log tailer and filter
│   └── (modules with a web UI also contain a `frontend/` dir, embedded via `//go:embed`)
├── web/
│   └── shared/frontend/       # Shared theme/common partials injected into all frontends
├── install.sh                 # Quick install script for the tool itself
├── go.mod / go.sum
├── renovate.json
├── AGENTS.md                  # Agent guidance for this project
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

### WOL Server (`internal/wol/command.go`)
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/wake/{hostname}` | Send WOL magic packet |
| POST | `/api/register` | Agent registration |
| GET/POST | `/api/aliases` | List / create MAC aliases |
| DELETE | `/api/aliases/{name}` | Delete alias |
| GET/POST | `/api/notify/{hostname}` | Boot/shutdown events |

### ES UI (`internal/es/command.go`)
| Method | Path | Purpose |
|---|---|---|
| GET | `/api/status` | ES connection status |
| GET | `/api/indices` | List ES indices |
| POST | `/api/search` | Execute ES query |
| GET/PUT | `/api/config` | View/update ES connection config |

### Mock Server (`internal/mock/mockserver.go`)
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/mock/query/{rs}` | Paginated mock data query |

### File Server (`internal/mock/fileserver.go`)
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/mock/file` | File upload |

---

## License

MIT
