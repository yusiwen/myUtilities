# metrics — Time-series metrics collection

Collect host metrics (CPU, memory, disk, load) as a long-running agent, store them
in a built-in time-series DB (BoltDB), and query them via CLI or HTTP API.

## Architecture

`mu metrics` has four roles:

- **`mu metrics serve`** — starts the HTTP server (receives agent reports, query API,
  compaction). Optionally `--agent` starts local collection in the same process
  (embedded mode, for single-machine use).
- **`mu metrics agent`** — collects local metrics; with `--server` it pushes to a
  remote server, otherwise it writes to a local BoltDB file.
- **`mu metrics query`** — queries stored metrics through the server's HTTP API
  (table / json / csv output).
- **`mu metrics compact`** — manually triggers data expiry/compaction.

```bash
# Start the metrics server (HTTP API on port 8096), optionally with built-in agent
mu metrics serve --port 8096 --agent --interval 30s

# Run a standalone agent that reports to a remote server
mu metrics agent --server http://metrics-host:8096 --interval 30s

# Query stored metrics (names: cpu.used.percent, memory.used.percent, load.1m, etc.)
mu metrics query cpu.used.percent --last 1h --format table
mu metrics query --list                 # list all metric names
mu metrics query load.1m --tags host=myhost --format json
mu metrics query cpu.used.percent --from 2024-01-01T00:00:00Z --to 2024-01-02T00:00:00Z

# Manually compact / expire old data
mu metrics compact --retention 30d
```

## HTTP API

The server exposes JSON APIs under `/api/metrics`:

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/metrics` | GET | List all metric names |
| `/api/metrics/hosts` | GET | List all known host tag values |
| `/api/metrics/{name}` | GET | Query data points |
| `/api/metrics/write` | POST | Agent reports data |
| `/api/metrics/compact` | POST | Trigger compaction |

### List hosts `GET /api/metrics/hosts`

Returns the unique `host` tag values seen across all stored series (from the series
metadata, so series written before the metadata bucket existed are only reflected
once new data arrives):

```json
["nuc12wski5", "prod-web-01"]
```

### Query `GET /api/metrics/{name}`

| Param | Description | Example |
|---|---|---|
| `from` | Start time (RFC3339) | `2024-01-01T00:00:00Z` |
| `to` | End time | `2024-01-02T00:00:00Z` |
| `tags` | URL encoded `k=v,k=v` | `host%3DHostA,cpu%3D0` |
| `limit` | Max points returned | `1000` |

Response:

```json
{
  "metric": "cpu.used.percent",
  "tags": {"host": "HostA", "cpu": "0"},
  "points": [
    [1704067200000000000, 45.2],
    [1704067230000000000, 46.1]
  ]
}
```

### List all metrics `GET /api/metrics`

```json
["cpu.used.percent", "memory.used.bytes", "disk.used.percent"]
```

### Agent report `POST /api/metrics/write`

```json
[
  {
    "metric": "cpu.used.percent",
    "tags": {"host": "HostA", "cpu": "0"},
    "time": 1704067200000000000,
    "value": 45.2
  },
  {
    "metric": "memory.used.bytes",
    "tags": {"host": "HostA"},
    "value": 8589934592
  }
]
```

- `time` is optional; when empty the server uses the current time.
- The `host` tag is injected automatically by the agent (default `os.Hostname()`,
  overridable via `--hostname` or the config file).

## Web UI

`mu metrics serve` also serves a charting dashboard at `/` (Svelte + Chart.js):
time-range selector (15m/1h/6h/24h), auto-refresh, per-metric line charts with
latest/max/min stats, and CPU/memory/load summary cards. The chart x-axis is pinned
to the selected time range (not auto-scaled to data), so gaps in collection show up
as empty segments on the axis.

A **host selector** in the toolbar filters the view:

- **All hosts** (default) — every card draws one colored line per host (legend shown,
  colors are consistent per host across cards). Stats reflect the most recent host.
- **A specific host** — cards draw a single line for that host, and a host chip is
  shown next to the metric name. The CPU/memory/load summary cards follow the selection.

```bash
mu metrics serve --agent --interval 30s   # then open http://localhost:8096
```

## Status

`mu metrics status` reports the running metrics processes and their state. Both
`mu metrics serve` and `mu metrics agent` create a Unix domain socket under the
fixed `/run/mu` directory (`/run/mu/metrics.sock` and `/run/mu/agent.sock`)
that answer a JSON status payload; `serve` also exposes the
same data over HTTP at `GET /api/metrics/info`. The socket directory is
traverseable by all local users (`0711`) and the socket itself world-connectable
(`0666`), so `mu metrics status` works for any user — the payload is the same
public status info already served over HTTP.

```bash
mu metrics status                    # read /run/mu/*.sock, HTTP fallback on :8096
mu metrics status --server http://host:8096   # remote server over HTTP
```

Discovery order: the `/run/mu` sockets first, then an HTTP fallback on the
port (covers older binaries without a socket, or processes running without
permission to create the socket directory). If nothing is running the command
prints `no running metrics server or agent found` and exits 1; when a socket
file exists but is not accessible (e.g. wrong permissions) the message explains
so. A running server or agent exits 0.

The reported **mode** identifies how each process runs:

| Mode | Meaning |
|---|---|
| `server` | `mu metrics serve` without an embedded agent |
| `server-with-agent` | `mu metrics serve --agent` (also how the gateway manages it) |
| `agent-local` | `mu metrics agent` writing to the local BoltDB |
| `agent-remote` | `mu metrics agent --server` pushing to a remote server |

Example output for a running server:

```
Config:
  mode             server-with-agent
  pid              1234
  started_at       2026-08-19T01:36:33+08:00
  version          v1.3.6.3
  config-dir       /etc/mu
  config file      /etc/mu/metrics-config.json (found)
  retention        0 (forever)
  compact_interval 1d
  collect_interval 30s
  hostname         GL-MT6000
  db-path          /etc/mu/metrics.db
  debug            false

Running:
  server           http://localhost:8096
  state            running (47 metrics)

DB:
  file             /etc/mu/metrics.db
  size             1.2 MB
  modified         2026-08-18 07:00:00 +0000
  series           47
  points           1,234,567
  hosts            GL-MT6000
  metrics          cpu.used.percent, memory.used.percent, ... (47 total)
```

When only an agent is running, `status` prints an `Agent:` section instead
(mode, pid, server/db-path, interval) and still exits 0. Configuration defaults
are documented in `mu metrics serve --help` / `mu metrics agent --help`.

### Gateway integration

The gateway exposes the dashboard at `/metrics/` and proxies the read-only API
endpoints (`GET /api/metrics`, `GET /api/metrics/hosts`, `GET /api/metrics/{name}`)
to a running `mu metrics serve` backend:

```bash
mu metrics serve --agent --interval 30s   # metrics backend on :8096
mu gateway --metrics-server http://localhost:8096
# open http://localhost:8080/metrics/
```

Only the GET read endpoints are forwarded — `POST /api/metrics/write` and
`POST /api/metrics/compact` are never exposed through the gateway. The default
backend is `http://localhost:8096` (override with `--metrics-server` or the
`MU_METRICS_SERVER` env var).

### Managed server (gateway default)

Since the gateway manages its own `mu metrics serve` subprocess by default, no
separate `metrics serve` is needed:

```bash
mu gateway --port 8080     # auto-starts a managed server on :8096
```

The managed subprocess runs `mu metrics serve --port <p> --agent` (same binary)
with the gateway's `--config-dir` passed through, so its config and DB live
alongside the gateway's other configs. The subprocess follows the gateway's
lifecycle and is controllable from the dashboard's admin control bar
(Start/Stop/Restart, status, pid, uptime, recent logs). If the port is already
served by a metrics server the gateway detects it and proxies directly
(`external` mode); if a non-metrics service occupies the port the dashboard
shows an `error`. See [gateway.md](gateway.md) for the flags and the admin API
routes.

### Config file location and DB path (`mu metrics serve`)

`mu metrics serve` accepts `--config-dir` and `--db-path`:

```bash
mu metrics serve --config-dir /etc/mu --port 8096
mu metrics serve --db-path /var/lib/mu/metrics.db
```

- `--config-dir` — directory holding `metrics-config.json` (instead of the
  default `~/.config/mu/`).
- `--db-path` — full path to the BoltDB file, overriding everything below.

The DB file path is resolved by priority:

| Priority | Source | Result |
|---|---|---|
| 1 | `--db-path` | used as-is (full file path) |
| 2 | config `data_dir` | `<data_dir>/metrics.db` |
| 3 | `--config-dir` | `<config-dir>/metrics.db` |
| 4 | default | `~/.local/share/mu/metrics/metrics.db` |

When the gateway manages the subprocess it passes its own `--config-dir`
through, so a gateway started with `--config-dir=/etc/mu` stores the metrics DB
at `/etc/mu/metrics.db`.

## Agent behavior

### Collection

The agent collects OS metrics on an interval (default 30s) and flushes them either
to a remote server or to the local BoltDB. Available metric names:

| Metric | Tags |
|---|---|
| `cpu.used.percent` | — |
| `cpu.per_cpu.percent` | `cpu=N` |
| `memory.used.percent` | — |
| `memory.used.bytes` | — |
| `disk.used.percent` | `mount=/`, `device=sda1` |
| `disk.io.bytes` | `device=sda`, `direction=read/write` |
| `net.io.bytes` | `interface=eth0`, `direction=in/out` |
| `load.1m` / `load.5m` / `load.15m` | — |

### Push failure handling

When the agent is configured with `server_url` and the server is unreachable, it
retries with **exponential backoff** (3 attempts: 1s → 2s → 4s). After the last
failed attempt the batch is dropped:

| Scenario | Result |
|---|---|
| Server healthy | Push to server |
| Server transient failure | Backoff retry 3 times (1s → 2s → 4s) |
| Server unreachable long-term | Batch dropped after retries (no local cache) |
| No server (pure local mode) | Writes directly to local BoltDB each time |

> Local buffering for `agent --server` (fall back to BoltDB while offline, then
> catch up) is planned but not yet implemented.

### Querying per host

The `host` tag is part of the tag hash that clusters series in BoltDB, so different
hosts land in different key spaces automatically. Querying without tags returns one
series per distinct host (grouped by tag hash, tags restored from series metadata):

```bash
# All CPU metrics for HostA
GET /api/metrics/cpu.used.percent?tags=host%3DHostA

# CPU metrics for all hosts (one series per host)
GET /api/metrics/cpu.used.percent

# HostA's CPU 0
GET /api/metrics/cpu.used.percent?tags=host%3DHostA,cpu%3D0
```

## Retention and compaction

Configured via `--retention` or the config file `retention` field. Supported
formats: `30d`, `7d`, `24h`, `90d`, `30m`. `0` = keep forever.

`--retention` semantics:

| `--retention` | Meaning |
|---|---|
| (empty, not passed) | Inherit the config file's `retention` |
| `0` | Keep forever, explicitly overriding the config file |
| `30d` / `24h` / etc. | Keep data at most this old, overriding the config file |

- **Auto**: `Compact(retention)` runs on serve/agent startup, then periodically
  every `compact_interval` (default `1d`, configurable in
  `metrics-config.json`). With `retention` unset/`0` no auto-compaction runs and
  data is kept forever.
- **Manual**: `mu metrics compact --retention 30d` triggers compaction immediately.

## Config

`~/.config/mu/metrics-config.json`:

```json
{
  "retention": "30d",
  "collect_interval": "30s",
  "compact_interval": "1d",
  "hostname": "Prod-Web-01",
  "server_url": "http://metrics-server:8096",
  "data_dir": "",
  "debug_log": false
}
```

- `retention` — data retention; `0`/unset = forever.
- `compact_interval` — how often auto-compaction runs (when `retention` is set);
  default `1d`.
- `hostname` — optional, default `os.Hostname()`; the agent injects a `host` tag on
  every write.
- `server_url` — used only by the agent; when empty the agent writes to local BoltDB.
- `data_dir` — optional, default `~/.local/share/mu/metrics/`.
- `debug_log` — enable debug logging (`--debug` flag also works).

Data files (defaults): DB at `~/.local/share/mu/metrics/metrics.db`; config at
`~/.config/mu/metrics-config.json`. Both can be relocated with `--config-dir` /
`--db-path` (see above).

## systemd

Example systemd service files are provided for running the server and agent as
long-running daemons:

- `mu-metrics-server.service` — runs `mu metrics serve` (HTTP API + dashboard on
  `:8096`).
- `mu-metrics-agent.service` — runs `mu metrics agent` pushing to
  `http://localhost:8096` every 30s. It uses a soft dependency (`Wants`+`After`)
  on the server, so a server restart does not interrupt collection and the agent
  falls back to its local BoltDB buffer while the server is unreachable.

Install and start both:

```bash
sudo cp mu-metrics-server.service mu-metrics-agent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now mu-metrics-server.service
sudo systemctl enable --now mu-metrics-agent.service
```

Tune retention/interval/hostname via `~/.config/mu/metrics-config.json` (loaded
from the service's `HOME=/root`), or edit `ExecStart` in the unit files.

Stop and disable both:

```bash
sudo systemctl disable --now mu-metrics-server.service mu-metrics-agent.service
```

**Coexistence with the gateway:** when the server runs under systemd it holds
`:8096`, so a gateway started with the default `--metrics-manage` detects the
external server and proxies to it directly (external mode) instead of spawning a
managed subprocess — no extra configuration needed. See [gateway.md](gateway.md)
for the flags.

Design details: [plan/metrics-module-plan.md](plan/metrics-module-plan.md).

## Appendix: Design notes

- **TSDB storage** — BoltDB, single `series` bucket. Key:
  `<metric_name>\x00<fnv64a(sorted_tags)>\x00<timestamp_be(8 bytes)>`, value is the
  float64 bits. The FNV-1a tag hash keeps identical metric+tags clustered; the
  big-endian timestamp keeps keys ordered for `Cursor.Seek()` range scans. Each point
  is ~40 bytes. Query scans by `metric_name + tags_hash` prefix.
- **Collector** — a `Collector` interface (`Name()` + `Collect(registry)`); OS metrics
  use gopsutil and update a go-metrics registry that is flushed on each tick.
- **Output persistence** — output/points are written in batches to avoid per-line DB
  writes.
- **Collection model** — v1 uses HTTP Push (agent → server). Future directions: Pull
  (Prometheus style) and WebSocket Pull (agent-initiated WS, server controls cadence,
  agent needs no open ports). See the plan for the full comparison.