# mu (myUtilities)

A multi-purpose CLI tool with subcommands for common development and operations tasks.

## Build

```bash
# Build for current platform (automatically builds Svelte frontend for WOL)
make darwin-arm64

# Build all common platforms
make all

# Build output is in bin/ directory
```

## Usage

```
mu <command> [subcommand] [flags]
```

### install — Install binaries from GitHub releases

```bash
mu install owner/repo --move
```

### mock — Mock servers for testing

```bash
mu mock mock-server --port 8081 --size 100
mu mock file-server --port 8082 --local-dir ./uploads
mu mock oauth-server --port 8083
```

### proxy — Database proxy with failover

```bash
mu proxy db --port 1521 \
  --route-name primary --db-host 10.0.0.1 --db-port 1521 \
  --route-name standby --db-host 10.0.0.2 --db-port 1521
```

### run — Execute commands with colored output

```bash
mu run --commands "echo hello" --commands "ls -la"
```

### wol — Wake-on-LAN HTTP server

Starts an HTTP server with a Svelte frontend and REST API for managing WOL aliases.

```bash
# Start server (interface name examples: br-lan on Linux, en0 on macOS, Ethernet0 on Windows)
mu wol serve en0 --port 8080

# List available network interfaces
mu wol interfaces
mu wol interfaces -v  # verbose output

# Run boot notification agent on remote machines
mu wol agent http://server:8080
```

#### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/aliases` | List all hostname→MAC mappings |
| `POST` | `/api/aliases` | Add/update alias (JSON: `{"name":"<host>","mac":"<mac>"}`) |
| `DELETE` | `/api/aliases/{name}` | Delete an alias |
| `POST` | `/api/wake/{hostname}` | Send WOL magic packet |
| `POST` | `/api/boot/{hostname}` | Record boot notification (agent) |
| `GET` | `/api/boot/{hostname}` | Query last boot time |
| `GET` | `/` | Svelte frontend UI |

Hostname must conform to RFC 952/1123. MAC must be in `xx:xx:xx:xx:xx:xx` format.
