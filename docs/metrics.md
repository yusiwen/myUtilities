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
| `/api/metrics/{name}` | GET | Query data points |
| `/api/metrics/write` | POST | Agent reports data |
| `/api/metrics/compact` | POST | Trigger compaction |

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
retries with **exponential backoff + local caching**:

| Scenario | Result |
|---|---|
| Server healthy | Push to server, no local write |
| Server transient failure | Backoff retry 3 times (1s → 2s → 4s) |
| Server unreachable long-term | Falls back to local BoltDB, no data loss |
| Server recovers | Next push returns to online mode and pushes directly |
| No server (pure local mode) | Writes directly to local BoltDB each time |

### Querying per host

The `host` tag is part of the tag hash that clusters series in BoltDB, so different
hosts land in different key spaces automatically:

```bash
# All CPU metrics for HostA
GET /api/metrics/cpu.used.percent?tags=host%3DHostA

# CPU metrics for all hosts
GET /api/metrics/cpu.used.percent

# HostA's CPU 0
GET /api/metrics/cpu.used.percent?tags=host%3DHostA,cpu%3D0
```

## Retention and compaction

Configured via `--retention` or the config file. Supported formats: `30d`, `7d`,
`24h`, `90d`. `0` (or unset) = keep forever.

- **Auto**: a `Compact(retention)` runs on serve/agent startup, then every hour.
  `0` disables it (permanent retention).
- **Manual**: `mu metrics compact --retention 30d` triggers compaction immediately.

## Config

`~/.config/mu/metrics-config.json`:

```json
{
  "retention": "30d",
  "collect_interval": "30s",
  "hostname": "Prod-Web-01",
  "server_url": "http://metrics-server:8096",
  "data_dir": "",
  "debug_log": false
}
```

- `retention` — data retention; `0`/unset = forever.
- `hostname` — optional, default `os.Hostname()`; the agent injects a `host` tag on
  every write.
- `server_url` — used only by the agent; when empty the agent writes to local BoltDB.
- `data_dir` — optional, default `~/.local/share/mu/metrics/`.
- `debug_log` — enable debug logging (`--debug` flag also works).

Data files: DB at `~/.local/share/mu/metrics/metrics.db`; config at
`~/.config/mu/metrics-config.json`.

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