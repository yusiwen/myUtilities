# mu (myUtilities)

A multi-purpose CLI tool with subcommands for common development and operations tasks.

## Build

```bash
# Build for current platform (automatically builds all Svelte frontends)
make darwin-arm64

# Build all common platforms
make all

# Build output is in bin/ directory
```

> **Note:** The build automatically compiles five Svelte frontends: WOL, ES, Mock Dynamic, QR Code,
> and JAR Analyzer. Ensure `npm` is installed before building.

## Usage

```
mu <command> [subcommand] [flags]
```

### install — Install binaries from GitHub releases

```bash
mu install owner/repo --move
```

### crypto — Cryptographic tools

Encrypt and decrypt data with various algorithms (AES, DES, 3DES, SM4), generate
secure random passwords, decode JWT tokens, and encode/decode data. Supports both CLI and web UI.

```bash
# Generate a random password (with options)
mu crypto passwd -l 32
mu crypto passwd -l 16 --no-digits --special

# AES encrypt
mu crypto aes -e --plain-key "mykey" --input "hello" --output-format hex

# AES decrypt
mu crypto aes -d --plain-key "mykey" --input "hex-encoded-data" --input-format hex

# Encode / decode (base64, hex, URL)
mu crypto encode --type base64 "hello"
mu crypto decode --type hex "68656c6c6f"

# JWT decode and verify
mu crypto jwt decode <token>
mu crypto jwt verify --key secret <token>

# Serve web UI (standalone)
mu crypto serve --port 8087
```

The web UI provides:
- **Password Generator** tab — configurable length, digits/special char toggles, one-click copy
- **Encrypt / Decrypt** tab — cipher selection (AES/DES/3DES/SM4), ECB/CBC mode, key/IV input
- **Encode / Decode** tab — base64, base64url, hex, URL encode/decode
- **JWT** tab — decode JWT tokens, verify HMAC signatures with auto-detected algorithm

### diff — Text comparison tool

Compare two files or text strings with a side-by-side diff viewer. Supports both CLI and web UI.

```bash
# Compare two files
mu diff file a.txt b.txt

# Compare text strings
mu diff text "old text" "new text"

# Serve web UI (standalone)
mu diff serve --port 8088
```

The web UI provides a full-page CodeMirror-based merge view with:
- Side-by-side editors with real-time diff highlighting
- File upload for both sides
- Synchronized scrolling between panes
- Auto-save to localStorage (content persists across page reloads)

### k8s — Kubernetes utilities

Generate and decode Kubernetes Opaque Secret YAML files. Values are automatically
base64-encoded for the `data` section. List Kubernetes resources from your cluster
using your kubeconfig (`~/.kube/config` by default).

```bash
# Generate a Secret YAML from CLI arguments
mu k8s secret my-app DB_HOST=localhost DB_PASSWORD=s3cret

# Read key=value pairs from a .env file
mu k8s secret my-app --from-env .env

# Pipe key=value pairs from stdin
cat .env | mu k8s secret my-app

# Output to file
mu k8s secret my-app KEY=val -o secret.yaml

# Decode an existing Secret YAML back to plaintext
mu k8s secret secret.yaml --decode

# List resources from the current kubeconfig context
mu k8s get pods
mu k8s get pods -n kube-system
mu k8s get nodes
mu k8s get deployments
mu k8s get services
mu k8s get configmaps
mu k8s get namespaces
mu k8s get statefulsets
mu k8s get daemonsets
mu k8s get ingresses
mu k8s get secrets
mu k8s get pods --context my-cluster
mu k8s get pods --kubeconfig /path/to/config

# Describe a resource in detail
mu k8s describe pod my-pod -n default
mu k8s describe node my-node
mu k8s describe deployment my-deploy -n default
mu k8s describe service my-svc -n default
mu k8s describe configmap my-cm -n default
mu k8s describe namespace kube-system
mu k8s describe secret my-secret -n default

# Serve web UI (standalone)
mu k8s serve --port 8089
```

```bash
# Serve web UI with pre-loaded kubeconfig
mu k8s serve --port 8089 --kubeconfig ~/.kube/config
```

The web UI provides:
- **Secret** tab — encode/decode Secret YAML in one place, with mode switch, .env file loading, copy/download
- **Resources** tab — connect to a Kubernetes cluster by uploading or pasting your kubeconfig,
  list pods, nodes, deployments, services, configmaps, namespaces, statefulsets, daemonsets, ingresses, and secrets with namespace filtering and context switching;
  click any resource name to view detailed describe information in a modal dialog
  (kubeconfig is persisted at `~/.config/mu/kubeconfigs.yaml`, supports multiple saved configs)

Supports `key=value` format with `#` comments and blank lines in env files.

### mock — Mock servers for testing

```bash
mu mock mock-server --port 8081 --size 100
mu mock file-server --port 8082 --local-dir ./uploads
mu mock oauth-server --port 8083
mu mock dynamic-server --config mock-config.json
```

#### dynamic-server — Configurable multi-endpoint mock with hot-reload and admin UI

```bash
mu mock dynamic-server --config mock-config.json
```

Starts a mock server with a **web admin UI** at `http://localhost:8084/__admin/` where you can
add, edit, delete, and save endpoints in real time — no restart needed.

##### Admin UI

Open `http://localhost:8084/__admin/` in your browser:

- **Table** of all endpoints (method, path, status, delay)
- **Add Endpoint** button to create new endpoints
- **Edit** / **Del** actions per endpoint
- **Save to Config** button persists all current endpoints to the config file

Endpoints created or modified via the UI take effect immediately on the next matching request.

##### Config file format

```json
{
  "port": 8084,
  "endpoints": [
    {
      "id": "a1b2c3",
      "method": "POST",
      "path": "/api/users",
      "status": 201,
      "delay": "500ms",
      "headers": { "X-Request-Id": "{{header.x-request-id}}" },
      "body": "{\"created\": true, \"name\": \"{{body.name}}\"}"
    },
    {
      "id": "d4e5f6",
      "method": "GET",
      "path": "/api/users/:id",
      "status": 200,
      "body": "{\"id\": \"{{path.id}}\", \"name\": \"User {{path.id}}\", \"page\": \"{{query.page}}\"}"
    }
  ]
}
```

The `body` field is always a raw response string (JSON or plain text).

**Features:**

| Feature | Description |
|---|---|
| Admin web UI | `GET /__admin/` — browser-based endpoint management |
| Hot-reload | Add/edit/delete endpoints at runtime without restart |
| Template variables | `{{path.id}}` `{{query.page}}` `{{header.authorization}}` `{{body.name}}` |
| Custom status code | Per-endpoint `"status": 201`, `404`, `500`, etc. |
| Custom headers | `"headers": {"X-Custom": "value"}` (supports template variables) |
| Delay simulation | `"delay": "2s"` / `"500ms"` / `"1.5s"` |
| Path parameters | `/api/users/:id` matches `/api/users/42`, param available as `{{path.id}}` |
| Persistence | "Save to Config" button writes all endpoints back to the config file |
| Verbose logging | `--verbose` flag prints request/response details to stdout |

**Template sources:**

| Source | Syntax | Example |
|---|---|---|
| URL path param | `{{path.xxx}}` | `/api/users/:id` → `{{path.id}}` |
| Query string | `{{query.xxx}}` | `?page=1` → `{{query.page}}` |
| Request header | `{{header.xxx}}` | `Authorization: Bearer x` → `{{header.authorization}}` |
| JSON body | `{{body.xxx}}` | `{"name":"alice"}` → `{{body.name}}` |
| Nested body | `{{body.x.y.z}}` | `{"user":{"name":"alice"}}` → `{{body.user.name}}` |

> **Note:** Conditional responses (`"responses"` array with `when`/`then`) are not yet
> supported via the admin UI but can still be added by editing the JSON config file
> manually (they will be preserved through save operations).

### gateway — Unified service portal

Serves multiple mu services under a single HTTP server with a landing page.

```bash
mu gateway --port 8080
```

By default, WOL and ES are loaded from `~/.config/mu/wol-config.json` and `~/.config/mu/es-config.json`.
Mock Dynamic can be added by creating `~/.config/mu/mock-config.json`:

```bash
echo '{"port":8084,"endpoints":[]}' > ~/.config/mu/mock-config.json
mu gateway --port 8080
```

| Route | Service | Description |
|---|---|---|---|
| `/` | Landing page | Card-based navigation to all services |
| `/wol/*` | Wake-on-LAN | WOL management frontend and API |
| `/es/*` | Elasticsearch | ES query frontend and API |
| `/mock/__admin/*` | Mock Dynamic | Dynamic mock endpoint management |
| `/qrcode/` | QR Code | QR code generator web UI |
| `/jarinfo/` | JAR Analyzer | JAR file analysis web UI |
| `/crypto/` | Crypto | Encrypt, decrypt, passwords, JWT, encode/decode |
| `/diff/` | Diff | Side-by-side text comparison |
| `/k8s/` | K8s | Kubernetes Secret YAML generator and decoder |

All services are optional — if a config file is missing (mock), the corresponding route is
skipped with a warning and the rest of the gateway starts normally.

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

### watch — Watch resources for changes

Monitor file systems and git remotes for changes, with real-time event output.

```bash
# Watch a directory for file changes (every 5s)
mu watch file ./src

# Custom interval with glob filtering
mu watch file . --interval 2s --include "*.go" --exclude "vendor/*"

# Watch git remote for upstream updates (every 60s)
mu watch git . --interval 30s --branch main
```

```
$ mu watch file ./src --interval 2s --include "*.go"
Watching /home/user/src for changes (interval: 2s)...
[2026-07-07 14:00:00] ADDED    src/main.go
[2026-07-07 14:00:05] MODIFIED src/utils.go
[2026-07-07 14:00:10] DELETED  src/old.go
```

Git authentication (env vars take priority over config):

```bash
export GIT_AUTH_USER="myuser"
export GIT_AUTH_PASS="ghp_xxx"
mu watch git . --interval 60s
```

Or configure in `~/.config/mu/watch.json`:

```json
{
  "git_auth": {
    "username": "myuser",
    "password": "ghp_xxx"
  }
}
```

### qrcode — Generate QR codes

Encode text or file content as a QR code. Output to terminal (Unicode), save as PNG, or
serve via web UI.

```bash
# Terminal output
mu qrcode gen "https://example.com"

# Pipe from stdin
cat xxxx.conf | mu qrcode gen
mu qrcode gen < xxxx.conf

# Save as PNG
mu qrcode gen -o qrcode.png "https://example.com"

# Error correction level
mu qrcode gen --level high "data"

# Serve web UI (standalone)
mu qrcode serve --port 8085
```

Verify decoded content:

```bash
sudo apt install zbar-tools
mu qrcode gen -o /tmp/qr.png "https://example.com"
zbarimg /tmp/qr.png
# QR-Code:https://example.com
```

### serve — Static file server

Start an HTTP static file server for a local directory. Useful for previewing static sites or sharing files over LAN.

```bash
# Serve current directory on port 8080
mu serve

# Serve a specific directory on a custom port
mu serve ./dist --port 3000

# Enable CORS for cross-origin requests
mu serve --cors

# Log requests to stderr
mu serve -v
```

```
$ mu serve ./dist --port 3000 --cors
Serving /home/user/project/dist on http://localhost:3000
```

### jar info — Analyze JAR files

Parse class file versions, MANIFEST.MF, Maven coordinates, and multi-release info from a JAR.
Supports CLI output and web UI (file upload).

```bash
# CLI analysis
mu jar info app.jar

# Serve web UI (standalone)
mu jar info serve --port 8086
```

```
$ mu jar info app.jar
Target JDK:     11
Classes:        342
Total entries:  512
Compressed:     1.2 MB → 2.8 MB (43%)
Manifest:
  Main-Class:            com.example.Main
  Created-By:            Apache Maven 3.9.6
  Build-Jdk:             17.0.8
  Implementation-Version: 2.1.0
  Automatic-Module-Name:  com.example.myapp
Maven:          com.example:my-app:1.2.3
Signed:         false
Multi-release:  true
  JDK 9:  8 classes
  JDK 11: 12 classes
Version breakdown:
  Java 8  (52):   322
  Java 11 (55):   20
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
