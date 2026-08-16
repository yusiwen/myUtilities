# mu (myUtilities)

A multi-purpose CLI tool with subcommands for common development and operations tasks.

![mu Gateway](docs/mu-gateway-screenshot.png)

## Features

| Command | Type | Description | Web UI |
|---|---|---|---|
| [install](docs/install.md) | DevOps | Install binaries from GitHub releases (asset search, tags, tokens) | – |
| [mock](docs/mock.md) | Testing | Mock servers: mock-server, file-server, oauth-server, dynamic-server with admin UI | ✓ |
| [qrcode](docs/qrcode.md) | Utils | Generate QR codes (terminal, PNG, web UI) | ✓ |
| [serve](docs/serve.md) | DevOps | Static file server with CORS and logging | – |
| [svcreg](docs/svcreg.md) | Ops | Service registry server (ServiceCenter-compatible) + dashboard | ✓ |
| [proxy](docs/proxy.md) | Ops | Database proxy with failover (Oracle) | – |
| [run](docs/run.md) | DevOps | Command runner with live display + YAML recipe orchestration | – |
| [wol](docs/wol.md) | Ops | Wake-on-LAN server, boot/shutdown agent, interface discovery | ✓ |
| [es](docs/es.md) | Data | Elasticsearch query tool (status, indices, search) | ✓ |
| [git](docs/git.md) | Dev | AI commit messages, multi-turn code review, .gitignore templates | – |
| [watch](docs/watch.md) | Ops | Watch filesystem / git remotes for changes | – |
| [k8s](docs/k8s.md) | Cloud | Kubernetes Secret YAML generator + resource browser | ✓ |
| [jar](docs/jar.md) | Dev | Analyze JAR files (JDK version, manifest, Maven coords) | ✓ |
| [gateway](docs/gateway.md) | Ops | Unified portal serving all web-enabled modules | ✓ |
| [diff](docs/diff.md) | Dev | Side-by-side text/file comparison | ✓ |
| [network](docs/network.md) | Net | DNS, DIG, and WHOIS lookups | ✓ |
| [misc](docs/misc.md) | Utils | JSON format/validate, UUID, timestamp, hash, trackers | ✓ |
| [crypto](docs/crypto.md) | Utils | Encrypt/decrypt, passwords, JWT, encode/decode | ✓ |
| [ask](docs/ask.md) | AI | Ask an LLM questions (optional web search) | – |
| [budget](docs/budget.md) | AI | Track LLM API usage/balance across providers | ✓ |
| [metrics](docs/metrics.md) | Ops | Time-series host metrics collection and querying | – |
| [scip](docs/git.md#scip-semantic-code-intelligence) | Dev | SCIP semantic code intelligence (indexers + index) | – |
| [fleet](docs/fleet.md) | DevOps | Remote batch execution: dispatcher + agents, file transfer | – |
| [completion](docs/completion.md) | Dev | Generate bash/zsh completion scripts | – |
| [set](docs/set.md) | Ops | Unified module configuration | – |

## Build

```bash
# Build for current platform (automatically builds all Svelte frontends)
make linux-amd64     # Linux x86_64
make linux-arm64     # Linux ARM64
make darwin-arm64    # macOS Apple Silicon
make windows-amd64   # Windows x86_64

# Build all common platforms
make all

# Quick build without frontends (no version injection)
make

# Build output is in bin/ directory
```

> **Note:** The build automatically compiles the Svelte frontends for all
> web-enabled modules (WOL, ES, Mock Dynamic, QR Code, JAR Analyzer, Crypto,
> Diff, K8s, Misc, Network, SvcReg, Budget). Ensure `npm` is installed before
> building.

## Usage

```
mu <command> [subcommand] [flags]
```
