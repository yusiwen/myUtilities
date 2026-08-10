# mu (myUtilities)

A multi-purpose CLI tool with subcommands for common development and operations tasks.

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

### ask — Ask LLM questions with optional web search

Sends a question to an OpenAI-compatible LLM API and returns a concise answer with reference URLs. Optionally fetches web search results via Brave Search API for up-to-date answers with source citations.

```bash
# Ask a question (uses LLM knowledge)
mu ask "What is a goroutine in Go?"

# With web search for real-time information
mu ask --search "What is WebAssembly?"
mu ask -s "Rust vs Go 2025 comparison"

# Chinese answer
mu ask --lang cn "什么是 WebAssembly？"

# Pipe input
echo "Explain TCP handshake" | mu ask

# Debug mode
mu ask --model gpt-4o --verbose "How does TLS work?"
```

Configuration at `~/.config/mu/ask-config.json`:

```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-xxx",
  "model": "gpt-4o-mini",
  "search_api_key": "BSA-xxx"
}
```

All settings can also be set via environment variables (`OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL`, `BRAVE_SEARCH_API_KEY`) or CLI flags.

### install — Install binaries from GitHub releases

```bash
mu install owner/repo --move
```

Install from a specific release tag, or search by program name:

```bash
mu install owner/repo@v1.2.0     # specific tag
mu install jq                     # auto-search GitHub for "jq"
```

Private repos and rate-limit avoidance via a GitHub token:

```bash
mu install owner/private-repo --token ghp_xxx
mu set installer --token ghp_xxx        # persist in ~/.config/mu/installer-config.json
mu set installer --unset                # remove the stored token
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

### budget — Query LLM API usage and balance

Track API usage and account balance across multiple LLM providers (DeepSeek, OpenRouter, Aliyun).
Supports both CLI and web UI, integrated into the gateway.

```bash
# Query all configured providers
mu budget balance

# Query a single provider
mu budget balance -p deepseek
mu budget balance -p openrouter -k sk-or-v1-xxx

# Query Aliyun balance + resource packages
mu budget balance -p aliyun

# Serve web UI (standalone)
mu budget serve --port 8095
```

Configuration at `~/.config/mu/budget-config.json`:

```json
{
  "providers": {
    "deepseek": {"api_key": "sk-xxx"},
    "openrouter": {"api_key": "sk-or-v1-xxx"},
    "aliyun": {
      "access_key_id": "LTAI5txxx",
      "access_key_secret": "xxx"
    }
  },
  "debug_log": false
}
```

The web UI (also available at `/budget/` in the gateway) displays balance cards for
each provider with real-time data. Aliyun cards additionally show resource package
details (CDN traffic, storage, CU packages) with remaining amounts and expiry dates.

Each provider card includes a **Top Up ↗** link that opens the provider's official
recharge page in a new tab. The default URL can be overridden via the optional
`"top_up_url"` field in the config:

```json
{
  "providers": {
    "deepseek": {
      "api_key": "sk-xxx",
      "top_up_url": "https://my-custom-portal.com/recharge"
    }
  }
}
```

| Provider | Auth Method | API Endpoint |
|----------|-------------|--------------|
| DeepSeek | `Bearer <API_KEY>` | `GET /user/balance` |
| OpenRouter | Management key → `GET /api/v1/credits`, fallback to `GET /api/v1/auth/key` | |
| Aliyun | AK/SK HMAC-SHA1 signature | `QueryAccountBalance` + `QueryResourcePackageInstances` |

API key resolution: `--key` flag → `budget-config.json → providers.<name>.api_key`.

Debug logging can be enabled with `"debug_log": true` — writes to `~/.config/mu/budget.log`.

### set — Unified module configuration

Configure any module's persistent settings via a single `mu set` command.
Each module implements the `ModuleSetter` interface and registers itself
at startup.

```bash
# List available modules
mu set

# Update ask config
mu set ask --config-api-key sk-xxx --config-base-url https://api.deepseek.com

# Update git commit config
mu set commit --config-model deepseek-v4-flash --config-api-key sk-xxx

# Update ES connection
mu set es --config-host http://localhost:9200 --config-username elastic

# Update service registry
mu set svcreg --config-host 0.0.0.0 --config-port 30100

# Update WOL settings
mu set wol --config-interface br-lan --config-port 8080 --config-token secret

# Update watch auth
mu set watch --config-git-user myuser --config-git-password ghp_xxx

# All flags support --config <path> to use a custom file
mu set es --config-host https://es.example.com --config /etc/mu/es-config.json
```

Available modules: ask, es, git, svcreg, watch, wol.

### es — Elasticsearch query tool

Query and explore Elasticsearch indices through a web UI. Connection settings are
persisted in `~/.config/mu/es-config.json`.

```bash
# Configure the ES connection (or use mu set es)
mu es set host http://localhost:9200
mu es set user elastic
mu es set password my-password

# Serve web UI (standalone)
mu es serve --port 8084
```

The web UI provides:
- **Status** — connection health check against the configured host
- **Indices** — browse index list with document counts
- **Search** — run arbitrary ES queries and view results

Also available in the gateway at `/es/`. Connection info is masked in config
displays (password never shown in plaintext).

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

# Show CPU/memory metrics (requires metrics-server)
mu k8s get nodes --metrics
mu k8s get pods --metrics -n default

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
  list pods, nodes, deployments, services, configmaps, namespaces, statefulsets, daemonsets, ingresses, and secrets with namespace filtering and context switching (with optional metrics for pods and nodes);
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

| Conditional responses | Per-endpoint `"responses"` with conditions using template expressions |
| Recursive conditions | Each condition can contain child conditions for multi-level branching |
| Default fallback | Conditionless `"default": true` response, or parent `body` as implicit fallback |

##### Conditional responses

Each endpoint can specify a list of `"responses"` evaluated in order. The first matching
condition wins; if none match, the endpoint's own fields serve as the fallback.

```json
{
  "method": "GET",
  "path": "/api/hello/:id",
  "status": 200,
  "delay": "100ms",
  "body": "{\"message\": \"Hello guest!\", \"id\": \"{{path.id}}\"}",
  "responses": [
    {
      "condition": "{{path.id}} == 1",
      "delay": "500ms",
      "headers": {"X-Role": "admin"},
      "body": "{\"message\": \"Hello Admin!\", \"role\": \"admin\"}"
    },
    {
      "condition": "{{path.id}} == 2",
      "body": "{\"message\": \"Hello User!\", \"role\": \"user\"}"
    }
  ]
}
```

**Operator reference:**

| Operator | Example | Effect |
|----------|---------|--------|
| *(none)* | `{{header.auth}}` | Exists / non-empty |
| `==` | `{{path.id}} == 1` | Equal |
| `!=` | `{{path.id}} != admin` | Not equal |
| `>` / `<` / `>=` / `<=` | `{{path.id}} > 100` | Numeric comparison |
| `contains` | `{{body.email}} contains @` | Substring match |
| `matches` | `{{path.id}} matches ^\\d+$` | Regex match |

##### Default demo endpoints

When the **gateway** (`mu gateway`) starts and `mock-config.json` does not exist yet,
it auto-creates two demo endpoints:

| Route | Description |
|-------|-------------|
| `GET /api/hello` | Simple greeting — quick sanity check |
| `GET /api/hello/:id` | Conditional response demo with path params, headers, delay |

Visit `http://localhost:8080/mock/api/hello` or `http://localhost:8080/mock/api/hello/1`
to try them.

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

### svcreg — Service Registry (ServiceCenter-compatible)

A lightweight RESTful server compatible with Apache ServiceComb ServiceCenter protocol.
Acts as a drop-in replacement for `service-center` — Java Chassis / Spring Cloud Huawei
clients can point `discovery.address` to `mu svcreg` and continue working unchanged.

Includes a Svelte 5 web dashboard (dashboard, services, instances, admin) with integrated
server lifecycle management.

```bash
# Start API-only server (bare, no frontend)
mu svcreg serve

# Start API + web dashboard (same port)
mu svcreg serve --web

# Start standalone web frontend (connects to a remote serve)
mu svcreg frontend --server http://192.168.1.10:30100 --port 30101

# Show server status
mu svcreg status

# List registered services
mu svcreg list services
mu svcreg list services --environment development

# List instances (single service or all)
mu svcreg list instances --service-id <id>
mu svcreg list instances --all
mu svcreg list instances --all --environment production
```

#### Web Dashboard

The web frontend (served at `serve --web` or standalone `frontend`) provides four tabs:

| Tab | Features |
|-----|----------|
| Dashboard | Server status, service/instance counts |
| Services | Filter by environment, expand for instance detail, delete |
| Instances | All instances across services |
| Admin | Start/stop serve subprocess, config port/host/DB path, live logs, independent process group option |

The Admin tab persists server state across restarts via PID file recovery.

#### Configuration

Server settings persisted in `~/.config/mu/svcreg-config.json`:

```json
{
    "host": "0.0.0.0",
    "port": 30100,
    "db_path": "~/.config/mu/svcreg.db"
}
```

CLI flags (`--host`, `--port`, `--db-path`) override the config file.

#### Supported API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v4/{project}/registry/version` | Server version + listen address |
| `GET` | `/v4/{project}/registry/health` | Cluster health |
| `GET` | `/v4/{project}/registry/existence` | Check service/schema existence |
| `POST` | `/v4/{project}/registry/microservices` | Register service |
| `GET` | `/v4/{project}/registry/microservices` | List services |
| `GET` | `/v4/{project}/registry/microservices/{id}` | Get service detail |
| `PUT` | `.../{id}/properties` | Update service properties |
| `DELETE` | `.../{id}` | Delete service (cascades instances + schemas) |
| `POST` | `.../{serviceId}/instances` | Register instance |
| `GET` | `.../{serviceId}/instances` | List instances |
| `DELETE` | `.../{serviceId}/instances/{id}` | Unregister instance |
| `PUT` | `.../instances/{id}/properties` | Update instance properties |
| `PUT` | `.../instances/{id}/status?value=` | Update instance status |
| `PUT` | `.../instances/{id}/heartbeat` | Single heartbeat |
| `PUT` | `/v4/{project}/registry/heartbeats` | Batch heartbeat |
| `GET` | `/v4/{project}/registry/instances` | Find instances |
| `POST` | `/v4/{project}/registry/instances/action?type=query` | Batch find (SDK fallback) |
| `GET` | `.../watcher` | WebSocket instance watch |
| `POST/GET/DELETE` | Tags + Schemas | Tag and schema management |

#### Storage

Uses BoltDB (embedded), no external dependencies. Instance TTL defaults to
30s interval × 3 retries (90s). Heartbeat via REST `PUT` extends the lease.

Logs every request with method, path, status code, and duration.

### gateway — Unified service portal

Serves multiple mu services under a single HTTP server with a landing page.

```bash
mu gateway --port 8080
mu gateway --config-dir /etc/mu    # custom config directory
```

By default, module configs are read from `~/.config/mu/`:

```
~/.config/mu/
├── wol-config.json
├── es-config.json
├── mock-config.json     (optional, auto-created on first start)
├── budget-config.json
├── ask-config.json
├── git-config.json
├── svcreg-config.json
└── watch.json
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
| `/misc/` | Misc | JSON, UUID, timestamp, hash tools |
| `/network/` | Network | DNS, DIG, and WHOIS query tools |
| `/svcreg/` | Service Registry | Register and discover microservices |
| `/budget/` | API Budget | Track LLM API balance across providers |

All services are optional — if a config file is missing (mock), the corresponding route is
skipped with a warning and the rest of the gateway starts normally.

### misc — Miscellaneous tools

JSON formatter/validator, UUID generator, timestamp converter, and hash calculator.
Supports both CLI and web UI.

```bash
# UUID generation
mu misc uuid
mu misc uuid 5

# JSON operations
mu misc json format '{"a":1}'
mu misc json validate '{"a":1}'
mu misc json minify '{ "a": 1 }'

# Timestamp conversion
mu misc timestamp              # current Unix time
mu misc ts "2025-01-01"        # date → Unix
mu misc ts 1735689600          # Unix → human date

# Hash computation
mu misc hash sha256 "hello"
mu misc hash md5 "hello"
mu misc hash sha512 -f file.txt

# Serve web UI (standalone)
mu misc serve --port 8090
```

The web UI provides:
- **JSON** tab — format, validate, and minify JSON
- **UUID** tab — generate UUID v4, single or batch
- **Timestamp** tab — auto-detect Unix timestamp or ISO date, real-time conversion
- **Hash** tab — compute MD5, SHA-256, SHA-512 hashes with file upload support

### network — Network tools

DNS lookup, DIG (detailed DNS query), and WHOIS lookup. Supports both CLI and web UI.

```bash
# DNS lookup
mu network dns example.com                # A record (default)
mu network dns example.com --type MX      # MX record
mu network dns example.com --type ALL     # All record types

# DIG (detailed query with full response)
mu network dig example.com                # dig-style output
mu network dig example.com --type MX
mu network dig example.com -n 8.8.8.8     # Specify nameserver

# WHOIS lookup
mu network whois example.com

# Serve web UI (standalone)
mu network serve --port 8091
```

The web UI provides:
- **DNS Lookup** tab — query various record types with TTL display
- **DIG** tab — full dig-style output with response headers, sections, and timing
- **WHOIS** tab — domain WHOIS lookup

### metrics — Time-series metrics collection

Collect host metrics (CPU, memory, disk, load) as a long-running agent, store them
in a built-in time-series DB (BoltDB), and query them via CLI or HTTP API.

```bash
# Start the metrics server (HTTP API on port 8096), optionally with built-in agent
mu metrics serve --port 8096 --agent --interval 30s

# Run a standalone agent that reports to a remote server
mu metrics agent --server http://metrics-host:8096 --interval 30s

# Query stored metrics (names: cpu.used.percent, memory.used.percent, load.1m, etc.)
mu metrics query cpu.used.percent --last 1h --format table
mu metrics query --list                 # list all metric names
mu metrics query load.1m --tags host=myhost --format json

# Manually compact / expire old data
mu metrics compact --retention 30d
```

The HTTP server exposes JSON APIs under `/api/metrics` (list, query, write,
compact) for the agent and CLI clients.

### completion — Generate shell completion script

Print a bash/zsh completion script for `mu` subcommands and flags. The scripts
dynamically parse `mu <path> --help` output, so they stay accurate as commands change.

```bash
# bash — add to ~/.bashrc
source <(mu completion bash)

# zsh — add to ~/.zshrc
source <(mu completion zsh)
```

### proxy — Database proxy with failover

```bash
mu proxy db --port 1521 \
  --route-name primary --db-host 10.0.0.1 --db-port 1521 \
  --route-name standby --db-host 10.0.0.2 --db-port 1521
```

### run — Execute commands with colored output

```bash
mu run --command "echo hello" --command "ls -la"
mu run --command "greet::echo hello"   # optional name prefix: <name>::<command>
```

### git — Git utilities with AI

Subcommands: `commit`, `review`, `ignore`.

#### git commit — AI-generated conventional commit messages

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

#### git review — AI-powered code review (Agent mode)

Analyzes local changes using a multi-turn LLM agent. The agent can read files, search
code, and inspect diffs before producing a structured markdown review.

```bash
# Review unstaged changes
mu git review

# Review staged changes
mu git review --staged

# Branch comparison
mu git review --base origin/main

# Compare two commits (see warning below)
mu git review --base <hashA> --target <hashB>

# Chinese output
mu git review --lang cn

# Extra context for the reviewer
mu git review --context "focus on error handling"

# List saved reviews (current project only)
mu git review --list

# List all saved reviews
mu git review --list --list-all

# Limit tool call rounds
mu git review --max-turns 10

# Disable SCIP semantic tools (e.g. on very large repos)
mu git review --no-scip

# Force regeneration of the SCIP index
mu git review --refresh-scip
```

> **`--target` consistency limitation:** `--base`/`--target` review committed
> ranges via `git diff <base>..<target>`. The agent's context tools (`read_file`,
> `read_function`, `search_code`, and the SCIP index) always operate on the
> **working tree**, which reflects the current `HEAD`. Reviewing a range whose
> `--target` is *not* the current `HEAD` would pair the target's diff with the
> HEAD's file/index content, producing potentially wrong results — `mu git
> review` therefore rejects it. To review an older target, first check it out
> (`git checkout <hash>`) then run `mu git review --base <hash>` (target
> defaults to `HEAD`). A dirty working tree during a committed-range review
> prints a warning.

> **Empty-diff hints:** like `git diff`, `git review` ignores untracked and
> staged files by default. When there is nothing to review it explains why and
> how to proceed: untracked files (`git add -N <file>` to include them), staged
> files (`mu git review --staged`, or `git reset` to unstage).

The review is rendered with syntax highlighting via `glamour`, paginated through
`less -R` (or `$PAGER`), and saved to:

```
~/.cache/mu/git_reviews/<project>_<branch>_<timestamp>.md
```

Saved files include YAML front matter with review metadata (commit, branch, diff stat,
strategy, timestamp, etc.).

#### SCIP semantic code intelligence

`git review` uses **SCIP** (Sourcegraph's language-agnostic symbol index, the successor
to LSIF) to give the review agent precise semantic understanding of the codebase.
The agent gains four semantic tools:

| Tool | Purpose |
|------|---------|
| `find_references` | All usages/call sites of a symbol across the repo — assess the impact of changing or deleting a function |
| `find_definition` | Jump to the definition of a symbol referenced in the diff |
| `symbol_info` | Hover-style signature, kind, and doc comment |
| `read_function` | Reads the exact enclosing function body (upgraded from a fixed ±30-line window) |

Indexers are installed **on demand, treesitter-nvim style**: on the first review the
language is auto-detected from the repo (e.g. `go.mod`, `pom.xml`, `Cargo.toml`), the matching
indexer binary is downloaded from a GitHub release into `~/.cache/mu/scip/tools/`, and the index
is built and cached per commit in `~/.cache/mu/scip/index/`. Dirty working trees use a
`working` index (rebuilt when source files changed). A stale index is rebuilt with an
explanatory line; building shows a spinner.

Indexer output is captured to a temp file (streamed live with `--verbose`); on a failed
build the error lines are extracted and the full log is kept, e.g.:
`Full indexer log kept at: /tmp/scip-index-XXX`. Go and Rust failures degrade gracefully to
text tools, while a **Java build failure aborts the review** (fail fast), since `scip-java`
runs a real Maven/Gradle build.

SCIP management commands:

```bash
# Install the indexer for a language (auto-downloaded)
mu scip install go
mu scip install java
mu scip install rust
mu scip install go --release v0.3.0   # install a specific release tag

# List available / installed indexers (configured / pinned / installed versions)
mu scip list

# Build the index for the current repo
mu scip index

# Update indexer(s) to the latest release (persists the version in config)
mu scip update go              # update a single language
mu scip update                 # update all enabled indexers
mu scip update --dry-run       # show old → new without downloading
mu scip update --no-pin        # download only, don't touch git-config.json
mu scip update --keep-old      # keep previous versions instead of removing them

# Remove all cached indexers and indexes
mu scip purge
```

**Indexer version control:** each language's indexer release tag is pinned in
code as a conservative default (e.g. scip-go `v0.2.7`, scip-java `v0.13.1`,
rust-analyzer `2026-08-03`).
`mu scip update` upgrades to the latest release and records the new tag in
`git-config.json` under `review.scip.versions`, so upgrades are explicit and
persistent. Overrides can also be set manually:

```bash
mu set git review --scip-version go=v0.3.0      # pin a specific release
mu set git review --scip-version-rm go          # remove the override
```

**Java projects:** `scip-java` indexes by actually running the Maven/Gradle build
(`clean verify -DskipTests` / `clean compileTestJava...`), so it needs:

- JDK 17+ (`java` on `PATH`)
- a Maven or Gradle project whose build can compile cleanly
- network access to resolve dependencies on the first run (takes minutes)

`git review` builds the Java index automatically when missing or stale (fail-fast on a
failed build — see the retained-log hint above); use `--refresh-scip` to force a rebuild.

**Rust projects:** `git review` uses `rust-analyzer scip` to index Cargo workspaces, so it needs:

- `cargo` and `rustc` on `PATH`
- a Cargo workspace that resolves cleanly (indexing loads the whole workspace)

The rust-analyzer release asset is a bare gzipped binary (not a tar archive) and has no
companion SHA256 checksum, so it is downloaded without checksum verification. `rust-analyzer`
builds its SCIP index via load-bearing inference; macro/generic-heavy code may resolve less
precisely than with `scip-go`.

Configuration lives in `git-config.json` under the `review.scip` key:

```json
{
  "review": {
    "provider": "default",
    "lang": "en",
    "scip": {
      "enabled": true,
      "auto_install": true,
      "cache_dir": "",
      "versions": { "go": "v0.3.0" }
    }
  }
}
```

`cache_dir` defaults to `~/.cache/mu/scip`. Disabling `enabled` or `auto_install` makes
reviews silently fall back to text tools.

#### git ignore — Download .gitignore templates

Downloads .gitignore templates from the [github/gitignore](https://github.com/github/gitignore) repository.

```bash
# List available templates
mu git ignore list

# Auto-detect language and download template
mu git ignore

# Download a specific template
mu git ignore Go

# Merge with existing .gitignore
mu git ignore Python --merge
```

### Configuration

LLM settings shared by `git commit` and `git review` are stored in `~/.config/mu/git-config.json`:

```json
{
  "providers": [
    {
      "name": "default",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "sk-xxx",
      "model": "deepseek-v4-flash"
    }
  ],
  "commit": {
    "provider": "default",
    "lang": "en"
  },
  "review": {
    "provider": "default",
    "lang": "cn",
    "reviews_dir": "~/.cache/mu/git_reviews"
  }
}
```

Configure via `mu set git`:

```bash
# Add a provider
mu set git provider add --name default --base-url <url> --api-key <key> --model <model>

# Remove a provider
mu set git provider rm --name <name>

# List providers
mu set git provider list

# Configure module defaults
mu set git commit --provider default --lang en
mu set git review --provider default --lang cn --reviews-dir ~/reviews
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
| `POST` | `/api/notify/{hostname}?type=boot` | Record boot notification and set status to "boot" |
| `POST` | `/api/notify/{hostname}?type=shutdown` | Record shutdown notification and set status to "shutdown" |
| `GET` | `/api/notify/{hostname}` | Query current status (boot/shutdown/unknown) and recent events |
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
