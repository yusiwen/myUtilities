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
mu mock dynamic-server --config mock-config.json
```

#### dynamic-server — Configurable multi-endpoint mock with hot-reload

```bash
mu mock dynamic-server --config mock-config.json
```

Define endpoints, response conditions, delays, and templates in a JSON config file:

```jsonc
{
  "port": 8084,
  "endpoints": [
    {
      "method": "POST",
      "path": "/api/users",
      "response": {
        "status": 201,
        "delay": "500ms",
        "headers": { "X-Request-Id": "{{header.x-request-id}}" },
        "body": "mock/create-user.json"
      }
    },
    {
      "method": "GET",
      "path": "/api/users/:id",
      "response": {
        "status": 200,
        "body": { "id": "{{path.id}}", "name": "User {{path.id}}", "page": "{{query.page}}" }
      },
      "responses": [
        { "when": { "path.id": "404" }, "then": { "status": 404, "body": { "error": "not found" } } },
        { "when": { "header.authorization": "" }, "then": { "status": 401, "body": { "error": "unauthorized" } } }
      ]
    },
    {
      "method": "GET",
      "path": "/api/users",
      "response": {
        "body": "mock/list-users.json"
      }
    }
  ]
}
```

**Features:**

| Feature | Description |
|---|---|
| Multi-endpoint | Any number of `method` + `path` combos in one config file |
| Template variables | `{{path.id}}` `{{query.page}}` `{{header.authorization}}` `{{body.name}}` |
| Custom status | `"status": 201`, `404`, `500`, etc. |
| Custom headers | `"headers": {"X-Custom": "value"}` (also supports templates) |
| Delay simulation | `"delay": "2s"` / `"500ms"` / `"1.5s"` |
| Conditional responses | Choose different `then` based on `when` conditions matching path, query, header, or body |
| Inline or file body | Body can be inline JSON or a file path (relative to config directory) |
| Hot-reload | Config and response files are read on each request — modify without restart |

**Template sources:**

| Source | Syntax | Example |
|---|---|---|
| URL path param | `{{path.xxx}}` | `/api/users/:id` → `{{path.id}}` |
| Query string | `{{query.xxx}}` | `?page=1` → `{{query.page}}` |
| Request header | `{{header.xxx}}` | `Authorization: Bearer x` → `{{header.authorization}}` |
| JSON body | `{{body.xxx}}` | `{"name":"alice"}` → `{{body.name}}` |
| Nested body | `{{body.x.y.z}}` | `{"user":{"name":"alice"}}` → `{{body.user.name}}` |

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

### git commit — AI-generated conventional commit messages

Generates a conventional commit message from staged changes using an LLM.

```bash
# Generate and confirm
mu git commit

# Skip confirmation (auto-commit)
mu git commit --yes

# Chinese commit message
mu git commit --lang cn

# Debug: print full prompt, API request/response, timing
mu git commit --verbose

# Dry run: print message without committing
mu git commit --dry-run
```

Configuration at `~/.config/mu/commit.json`:

```json
{
  "base_url": "https://api.deepseek.com/v1",
  "api_key": "sk-xxx",
  "model": "deepseek-v4-flash"
}
```

### wol — Wake-on-LAN HTTP server

Starts an HTTP server with a Svelte frontend and REST API for managing WOL aliases and tracking host status (boot/shutdown).

```bash
# Start server (interface name examples: br-lan on Linux, en0 on macOS, Ethernet0 on Windows)
mu wol serve en0 --port 8080

# List available network interfaces
mu wol interfaces
mu wol interfaces -v  # verbose output
```

#### Configuration

WOL settings are persisted in `~/.config/mu/wol-config.json`.

```bash
# Set agent server URL (used by mu wol agent when no URL is given)
mu wol set server http://192.168.1.100:8080

# Set network interface for the WOL server
mu wol set interface br-lan

# Set HTTP server port
mu wol set port 8080

# Set BoltDB file path
mu wol set db-path ~/.config/mu/bolt.db

# Set API auth token
mu wol set token my-secret-token

# Set hostname for agent registration
mu wol set hostname my-machine
```

#### Agent Notifications

Send boot/shutdown events or register this machine on the WOL server. The server URL can be given inline or set once via `mu wol set server` and omitted afterwards.

```bash
# Register this machine (stores hostname→MAC mapping)
mu wol agent --register http://192.168.1.100:8080

# Same, using server URL from config
mu wol agent --register

# Send boot notification
mu wol agent --boot http://192.168.1.100:8080

# Send shutdown notification (from config)
mu wol agent --shutdown
```

Flags `--register`, `--boot`, and `--shutdown` are mutually exclusive.

#### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/register` | Register agent: store hostname→MAC mapping (JSON: `{"name":"<host>","mac":"<mac>"}`) |
| `GET` | `/api/aliases` | List all hostname→MAC mappings |
| `POST` | `/api/aliases` | Add/update alias (JSON: `{"name":"<host>","mac":"<mac>"}`) |
| `DELETE` | `/api/aliases/{name}` | Delete an alias |
| `POST` | `/api/wake/{hostname}` | Send WOL magic packet |
| `POST` | `/api/boot/{hostname}` | Record boot notification and set status to "boot" |
| `GET` | `/api/boot/{hostname}` | Query last boot time |
| `POST` | `/api/shutdown/{hostname}` | Record shutdown notification and set status to "shutdown" |
| `GET` | `/api/shutdown/{hostname}` | Query current status (boot/shutdown/unknown) |
| `GET` | `/` | Svelte frontend UI |

Hostname must conform to RFC 952/1123. MAC must be in `xx:xx:xx:xx:xx:xx` format.

#### systemd Integration

Example oneshot service files are provided for sending boot/shutdown notifications automatically.

**Boot** — `wol-agent-boot.service`: fires after network is online, before user login. Edit `ExecStart` to match your server and hostname, then:

```bash
sudo cp wol-agent-boot.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now wol-agent-boot.service
```

**Shutdown** — `wol-agent-shutdown.service`: fires only on actual system halt/poweroff/reboot. It uses `DefaultDependencies=no` + `Before=shutdown.target` to ensure the network is still available when the notification is sent. Unlike `ExecStop` in a combined unit, it cannot be triggered by a manual `systemctl stop`. Edit `ExecStart`, then:

```bash
sudo cp wol-agent-shutdown.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable wol-agent-shutdown.service
```
