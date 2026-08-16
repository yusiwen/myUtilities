# mu metrics module plan

## Overall architecture

```
mu metrics
  ├── serve    HTTP server (receives agent reports + query API + frontend)
  ├── agent    collection daemon (collects on an interval, writes to local bbolt or pushes to a remote server)
  └── compact  manual compaction / expiry of old data
```

### Two roles

- **`mu metrics serve`** — starts the HTTP server; optional `--agent` also starts local collection (embedded mode, for single-machine use)
- **`mu metrics agent`** — collects local metrics; with `--server` it pushes to a remote server, otherwise it writes to local bbolt

## Directory structure

```
core/metrics/
  ├── model.go           Metric / DataPoint / WriteRequest types
  ├── tsdb.go            bbolt write / query / compact
  ├── tsdb_test.go
  └── collector/
      ├── collector.go   Collector interface + go-metrics Registry integration
      └── os.go          gopsutil → Gauge updates

metrics/
  ├── command.go         mu metrics serve / agent entry points
  ├── options.go         option structs
  └── config.go          metrics-config.json loading
```

---

## 1. Core data model — `core/metrics/model.go`

```go
type DataPoint struct {
    Timestamp int64   // UnixNano
    Value     float64
}

type Metric struct {
    Name   string            `json:"metric"`
    Tags   map[string]string `json:"tags"`
    Points []DataPoint       `json:"points"`
}

type WriteRequest struct {
    Metric    string            `json:"metric"`
    Tags      map[string]string `json:"tags"`
    Timestamp int64             `json:"time,omitempty"` // UnixNano; when empty the server uses the current time
    Value     float64           `json:"value"`
}
```

---

## 2. TSDB engine — `core/metrics/tsdb.go`

Uses `go.etcd.io/bbolt` (migration from `coreos/bbolt` already complete), with a dedicated database file.

### bbolt Key design

```
Bucket: "series"
  Key: <metric_name>\x00<fnv64a(sorted_tags)>\x00<timestamp_be(8 bytes)>
  Value: <float64bits(value)(8 bytes)>
```

- `fnv64a(sorted_tags)` = FNV-1a 64bit hash of sorted `k1=v1,k2=v2` → identical metric+tags cluster together
- `timestamp_be` = big-endian int64 UnixNano → ordered, supports `Cursor.Seek()` range scans
- A single record is ~40 bytes; million-level data volumes are no problem for bbolt

### Interface

```go
type DB struct { db *bolt.DB }

func Open(path string) (*DB, error)

// Write writes a single data point
func (db *DB) Write(name string, tags map[string]string, ts time.Time, value float64) error

// WriteBatch writes in batch (used by the agent on each flush)
func (db *DB) WriteBatch(metrics []Metric) error

// Query queries by metric + tags + time range
func (db *DB) Query(name string, tags map[string]string, from, to time.Time, limit int) ([]Metric, error)

// ListMetrics returns all metric names
func (db *DB) ListMetrics() ([]string, error)

// Compact deletes data points before cutoff (skipped when retention<=0)
func (db *DB) Compact(retention time.Duration) error
```

### Compact implementation strategy

```go
func (db *DB) Compact(retention time.Duration) error {
    if retention <= 0 {
        return nil  // keep forever
    }
    cutoff := time.Now().Add(-retention)
    cutoffBE := uint64ToBE(uint64(cutoff.UnixNano()))

    return db.Update(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("series"))
        c := b.Cursor()
        for k, _ := c.First(); k != nil; k, _ = c.Next() {
            // Key format: <name>\x00<hash>\x00<timestamp_be(8 bytes)>
            // timestamp_be is the last 8 bytes
            ts := k[len(k)-8:]
            if bytes.Compare(ts, cutoffBE) < 0 {
                if err := c.Delete(); err != nil {
                    return err
                }
            }
        }
        return nil
    })
}
```

---

## 3. Collector — `core/metrics/collector/`

### Collector interface

```go
type Collector interface {
    Name() string
    Collect(r metrics.Registry) error
}
```

**Role of the go-metrics Registry**: a current-value tree held in the agent process; each collection tick updates all Gauges, then flushes to bbolt.

### OS metric collection list — `os.go`

Depends on `github.com/shirou/gopsutil/v4`.

| Metric | Tags | Source |
|--------|------|--------|
| `cpu.used.percent` | — | gopsutil `cpu.Percent(0, false)` |
| `cpu.per_cpu.percent` | `cpu=N` | gopsutil `cpu.Percent(0, true)` |
| `memory.used.percent` | — | gopsutil `mem.VirtualMemory().UsedPercent` |
| `memory.used.bytes` | — | same `.Used` |
| `disk.used.percent` | `mount=/`, `device=sda1` | gopsutil `disk.Usage()` |
| `disk.io.bytes` | `device=sda`, `direction=read/write` | gopsutil `disk.IOCounters()` |
| `net.io.bytes` | `interface=eth0`, `direction=in/out` | gopsutil `net.IOCounters()` |
| `load.1m` / `load.5m` / `load.15m` | — | gopsutil `load.Avg()` |

At collection time gopsutil is called to get the values, then the corresponding `Gauge` / `GaugeFloat64` in the go-metrics Registry is updated.

---

## 4. CLI commands — `metrics/`

### Option definitions — `options.go`

```go
type ServeOptions struct {
    Port      int    `help:"HTTP API port." default:"8096"`
    Retention string `help:"Data retention (e.g. 30d, 7d, 0=forever)." default:"0"`
    Agent     bool   `help:"Also run agent locally."`
    Interval  string `help:"Collect interval (only with --agent)." default:"30s"`
}

type AgentOptions struct {
    Server    string `help:"Metrics server URL to report to." default:""`
    Interval  string `help:"Collect interval." default:"30s"`
    Hostname  string `help:"Override hostname for tags." default:""`
    Retention string `help:"Local data retention (when no server)." default:"0"`
}
```

### Example config — `metrics-config.json`

```json
{
  "retention": "30d",
  "collect_interval": "30s",
  "hostname": "Prod-Web-01",
  "server_url": "http://metrics-server:8096"
}
```

- `"retention"` supported formats: `"30d"`, `"7d"`, `"24h"`, `"90d"`. `"0"` or unset = keep forever.
- `"hostname"` optional, defaults to `os.Hostname()`. The agent injects a `host` tag on every write.
- `server_url` is used only by the agent; when empty it writes to local bbolt.

### Main entry — `command.go`

```go
type MetricsCmd struct {
    Serve ServeOptions `cmd:"" help:"Start metrics HTTP server."`
    Agent AgentOptions `cmd:"" help:"Start metrics collection agent."`
}
```

Registered in `myutilities.go`:
```go
Metrics MetricsCmd `cmd:"" name:"metrics" help:"Time-series metrics collection and querying."`
```

### Concrete behavior

```bash
# Start server (HTTP API, receives agent reports + query + compact)
mu metrics serve --port 8096 --retention 30d

# Start agent (collect local metrics → report to server)
mu metrics agent --server http://metrics-server:8096 --interval 30s

# Start agent+server combined (embedded mode, single-machine use)
mu metrics serve --agent --interval 30s --retention 30d

# Manually trigger compaction (without starting agent/server)
mu metrics compact --retention 30d
```

---

## 5. HTTP API — `serve` mode

| Endpoint | Method | Description |
|------|------|------|
| `/api/metrics` | `GET` | List all metric names |
| `/api/metrics/:name` | `GET` | Query data points |
| `/api/metrics/write` | `POST` | Agent reports data |

### Query `GET /api/metrics/:name`

Parameters:

| Param | Description | Example |
|------|------|------|
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

- `time` is optional; when empty the server uses the current time
- The `host` tag is injected automatically by the agent, defaulting to `os.Hostname()`, overridable via `--hostname` or the config file

---

## 6. Agent collection loop

```go
func (a *Agent) Run(ctx context.Context) error {
    registry := metrics.NewRegistry()  // go-metrics
    collectors := []Collector{
        collector.NewOSCollector(),
    }

    interval, _ := time.ParseDuration(a.cfg.Interval)
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    // first collection
    a.collectAndFlush(ctx, registry, collectors)

    for {
        select {
        case <-ticker.C:
            a.collectAndFlush(ctx, registry, collectors)
        case <-ctx.Done():
            return nil
        }
    }
}

func (a *Agent) collectAndFlush(ctx context.Context, r metrics.Registry, collectors []Collector) {
    ts := time.Now()

    // 1. collect → update go-metrics Gauges
    for _, c := range collectors {
        c.Collect(r)
    }

    // 2. flush to bbolt or remote server
    var batch []Metric
    r.Each(func(name string, i interface{}) {
        switch m := i.(type) {
        case metrics.GaugeFloat64:
            batch = append(batch, Metric{
                Name: name,
                Tags: a.hostTag,
                Points: []DataPoint{{Timestamp: ts.UnixNano(), Value: m.Value()}},
            })
        case metrics.Gauge:
            batch = append(batch, Metric{
                Name: name,
                Tags: a.hostTag,
                Points: []DataPoint{{Timestamp: ts.UnixNano(), Value: float64(m.Value())}},
            })
        }
    })

    if a.serverURL != "" {
        a.pushToServer(ctx, batch)
    } else {
        a.tsdb.WriteBatch(batch)
    }
}
```

### Server push failure handling

When the agent is configured with `server_url` to push to a remote server and that server is unreachable, it uses an **exponential backoff retry + local cache** strategy:

```go
const maxRetries = 3

func (a *Agent) pushToServer(ctx context.Context, batch []Metric) {
    var reqs []WriteRequest
    for _, m := range batch {
        for _, p := range m.Points {
            reqs = append(reqs, WriteRequest{
                Metric: m.Name, Tags: m.Tags,
                Timestamp: p.Timestamp, Value: p.Value,
            })
        }
    }
    data, _ := json.Marshal(reqs)

    url := a.cfg.serverURL + "/api/metrics/write"
    backoff := 1 * time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        if attempt > 0 {
            time.Sleep(backoff)
            backoff *= 2
        }
        resp, err := http.Post(url, "application/json", bytes.NewReader(data))
        if err == nil {
            resp.Body.Close()
            return // push succeeded
        }
        log.Printf("Push to server failed (attempt %d/%d): %v", attempt+1, maxRetries, err)
    }

    // final failure → write to local bbolt cache to avoid data loss
    log.Printf("Server unreachable, caching locally")
    if a.cfg.tsdb != nil {
        if err := a.cfg.tsdb.WriteBatch(batch); err != nil {
            log.Printf("Local cache write error: %v", err)
        }
    }
}
```

**Behavior:**

| Scenario | Result |
|------|------|
| Server healthy | Push to server, no local write |
| Server transient failure | Backoff retry 3 times (1s → 2s → 4s) |
| Server unreachable long-term | Falls back to local bbolt, no data loss |
| Server recovers | Next push returns to online mode, pushes directly |
| No server (pure local mode) | Writes directly to local bbolt each time |

### Agent auto-injected `host` tag

- Default: `os.Hostname()` automatically
- Overridable: `--hostname my-custom-name` or the `hostname` field in `metrics-config.json`
- On write the agent injects `host:<hostname>` into every data point's tags

### Distinguishing hosts at query time

```bash
# All CPU metrics for HostA
GET /api/metrics/cpu.used.percent?tags=host%3DHostA

# CPU metrics for all hosts
GET /api/metrics/cpu.used.percent

# HostA's CPU 0
GET /api/metrics/cpu.used.percent?tags=host%3DHostA,cpu%3D0
```

Because `fnv64a(sorted_tags)` in the bbolt key includes `host`, data from different hosts automatically lands in different key spaces; queries scan by the `metric_name + tags_hash` prefix to distinguish them.

---

## 7. Compaction (data expiry)

### Configuration

```json
{
  "retention": "30d"
}
```

- `"0"`, `""`, or unset → keep forever
- `"30d"` → keep 30 days
- `"7d"` → keep 7 days
- `"24h"` → keep 24 hours

### Automatic trigger

A `Compact(retention)` runs once when the Agent or Server starts. Afterwards it runs automatically every hour. `Compact(0)` does nothing (permanent retention).

### Manual trigger

```bash
mu metrics compact --retention 30d
```

---

## 8. Data file paths

- **Database file**: `~/.local/share/mu/metrics/metrics.db`
- **Config file**: `~/.config/mu/metrics-config.json`

---

## 9. New dependencies

```
go.etcd.io/bbolt v1.3.11         (migration complete)
github.com/rcrowley/go-metrics    → in-memory metric Registry
github.com/shirou/gopsutil/v4     → OS metric collection
```

---

## 10. Implementation order

| Phase | Content | Estimated changes |
|------|------|-----------|
| 1 | `core/metrics/model.go` + `tsdb.go` (Write/Query/Compact/ListMetrics) | ~300 lines |
| 2 | `core/metrics/collector/` (collector.go + os.go, go-metrics integration) | ~200 lines |
| 3 | `metrics/options.go` + `config.go` + `command.go` (serve subcommand + HTTP API) | ~250 lines |
| 4 | `metrics/command.go` (agent subcommand + collection loop + flush + compact) | ~200 lines |

---

## 11. Collection model design

### 11.1 Comparison of three collection models

#### HTTP Push (current agent mode)

```
Agent (timed tick)
  └── POST /api/metrics/write ──→ Server (receives and writes to bbolt)
```

| Feature | Description |
|------|------|
| Agent port | No port exposure needed |
| Data flow | Agent → Server (one-way) |
| Data cache | Agent-local bbolt (fallback on failure) |
| Offline handling | Agent retries, eventually falls back to local cache |
| Server pressure | Passively receives, does not control collection cadence |

#### HTTP Pull (Prometheus / node-exporter style)

```
Server (timed tick)
  └── GET http://agent:9100/metrics ──→ Agent (collects live and returns)
                                            └─ writes to Server bbolt
```

| Feature | Description |
|------|------|
| Agent port | **Needs an exposed port**, or be reachable by the Server |
| Data flow | Server → Agent (Server pulls) |
| Data cache | Agent needs no persistence at all |
| Offline handling | Server notices the pull failure and skips that tick |
| Server pressure | Actively controls collection cadence, manages all agent timers |

#### WebSocket Pull (compromise; recommended future direction)

```
Agent (initiates WS connection)
  │  ws://server:8096/ws/metrics
  │
Server ←── connection established ──→ Agent (zero port exposure)
  │                       │
  │  ──{"type":"collect"}──►(Server controls cadence)
  │  ◄──{"type":"metrics"}──(Agent collects live and returns)
  │                       │
  │  ──{"type":"collect"}──►
  │  ◄──{"type":"metrics"}──
```

| Feature | Description |
|------|------|
| Agent port | No port exposure needed |
| Data flow | Agent initiates the WS connection; Server sends pull commands over it (logically Pull) |
| Data cache | Agent needs no local persistence |
| Offline handling | Agent reconnects with exponential backoff; Server marks offline on disconnect |

### 11.2 WebSocket Pull detailed design

#### WS protocol messages

**Request/response synchronous semantics:** WS itself is full-duplex and asynchronous, but the Server guarantees serialization — **the Server does not send the next `collect` until it receives the `metrics` response for the previous one**. Each message carries an `id` field to correlate requests and responses and to detect out-of-order delivery and timeouts.

All integer values are represented as JSON numbers; timestamps are UnixNano.

```
Agent → Server (first message, registration):
{
  "type": "hello",
  "hostname": "agent-a",
  "capabilities": ["os"]       // list of capabilities this agent supports
}

Server → Agent:
{
  "type": "collect",
  "id": 1                      // incrementing request ID
}
{
  "type": "collect_interval",
  "interval": 30               // dynamically set the collection interval (seconds)
}

Agent → Server (collect response):
{
  "type": "metrics",
  "id": 1,                     // corresponds to the collect id
  "data": [
    {"metric":"cpu.used.percent","tags":{"host":"agent-a"},"value":45.2,"time":1704067200000000000}
  ]
}

Agent → Server (error report):
{
  "type": "error",
  "id": 1,                     // corresponds to the collect id; 0 for non-collect scenarios
  "message": "collection failure reason"
}
```

**Example interaction sequence:**

```
Server                              Agent
  │                                   │
  │──{"type":"collect","id":1}───────►│
  │                                   │ (Agent collects ~50ms)
  │◄──{"type":"metrics","id":1,       │
  │        "data":[...]}              │
  │                                   │
  │──{"type":"collect","id":2}───────►│
  │◄──{"type":"metrics","id":2,       │
  │        "data":[...]}              │
```

After sending `collect` the Server starts a timeout timer (e.g. 10s). If the `metrics` for the corresponding `id` is not received within the timeout, the Server closes that agent's connection and waits for the agent to reconnect.

#### Agent reconnect with backoff

Exponential backoff + jitter, capped at 60s:

```
attempt 1 → connect immediately
   failure → wait 1s (+ random 0~500ms)
attempt 2 → reconnect
   failure → wait 2s (+ random 0~500ms)
attempt 3 → reconnect
   failure → wait 4s (+ random 0~500ms)
  ...
attempt N → reconnect
   failure → wait 60s (cap)
```

#### Heartbeat keep-alive

```
Server → Agent: WebSocket Ping (every 30s)
Agent  → Server: WebSocket Pong
```

N consecutive Pong timeouts (e.g. 3) → the Server disconnects and marks the agent offline.

#### Server-side agent management

```
AgentManager
  ├── agents: map[hostname]*AgentConn
  │     ├── conn    *websocket.Conn
  │     ├── caps    []string           // capabilities
  │     ├── ticker  *time.Ticker       // sends collect per interval
  │     └── lastSeen time.Time
  ├── Register(conn, hostname, caps)
  ├── Unregister(hostname)
  └── List() → []AgentInfo
```

After each agent connects, the server creates a timer for it that sends `collect` commands per interval. When the agent returns `metrics`, the server writes them to local bbolt.

#### Data integrity strategy

Options for handling data during disconnects:

| Approach | Description | Complexity |
|------|------|--------|
| **Discard** (recommended) | Losing a few ticks during network jitter is fine; metrics are sampled data | Lowest |
| Agent in-memory cache | Ring buffer keeps the last N ticks, replay after reconnect | Medium |
| Agent-local bbolt | Like current push, writes to a local file, syncs after reconnect | High |

#### Pros and cons of WebSocket Pull

| Pros | Cons |
|------|------|
| Agent has zero port exposure | Server must maintain all agents' connection goroutines + timers |
| Server controls collection cadence | After a Server restart all agents must reconnect |
| Natural offline detection (WS disconnect is immediately known) | Harder to debug (cannot directly `curl` the data) |
| Suits dynamic environments (NAT/firewalls) | WS has frame control/heartbeat/encode-decode, more complex than HTTP GET |
| More network-efficient (no repeated HTTP handshakes) | Proxies/load balancers need extra long-connection timeout config |

### 11.3 Roadmap

```
v1 (current)   → HTTP Push (agent pushes to server on a timer, local bbolt cache)
v2 (future)    → WebSocket Pull (agent connects directly to server, server controls collection cadence)
v3 (future)    → WebSocket agent embedded in server (`mu metrics serve --agent` connects via WS directly)
```

---

## 12. Future extensibility (not in the current version)

| Feature | Timing |
|------|------|
| Docker container metrics collection (cgroup or Docker SDK) | Second release |
| Agent token auth (`--token` + server validation + auto host binding) | When security is required |
| Web frontend charts (gateway integration `/metrics/`) | When centralized display is needed |
| Aggregation queries (`avg`/`max`/`min`/`sum` + `window`) | When query requirements are clear |
| Prometheus remote write compatibility | When Grafana integration is needed |

---

## 13. Terminology

| Term | Meaning |
|------|------|
| **metric** | Metric name, e.g. `cpu.used.percent` |
| **tags** | Tag key-value pairs distinguishing dimensions of the same metric, e.g. `{host: HostA, cpu: 0}` |
| **data point** | A timestamp + value pair |
| **agent** | The collection side, running on the monitored machine |
| **server** | The storage + query side, receiving agent-reported data |
| **retention** | Data retention duration |
| **compaction** | The operation of deleting expired data |

---

## 14. Implementation progress

| Phase | Content | Status |
|------|------|------|
| 1 | `core/metrics/model.go` + `tsdb.go` (Write/Query/Compact/ListMetrics) | ✅ |
| 2 | `core/metrics/collector/` (collector.go + os.go, go-metrics integration) | ✅ |
| 3 | `metrics/options.go` + `config.go` + `command.go` (serve subcommand + HTTP API) | ✅ |
| 4 | Register `metrics` command in `myutilities.go`, project compiles | ✅ |
| 5 | `core/metrics/tsdb_test.go` (all 9 tests pass) | ✅ |
| 6 | `metrics/options.go` adds `QueryOptions` + `query` subcommand | ✅ |
| 7 | `metrics/command.go` adds `query.Run()` (table/json/csv three formats) | ✅ |
| 8 | Agent push retry + local cache strategy implemented | ✅ |
| 9 | Debug logging (`--debug` flag + `debug_log` config + key-path logs) | ✅ |
| 10 | `query` and `compact` now interact with the server via HTTP API (no longer read local bbolt) | ✅ |
| 11 | TSDB `Query` fix: empty tags scans all tag combinations under the metric (not just tag-less data) | ✅ |

### Created file list

```
core/metrics/
  ├── model.go             data type definitions
  ├── tsdb.go              core TSDB engine
  ├── tsdb_test.go         9 test cases
  └── collector/
      ├── collector.go     Collector interface
      └── os.go            OS metric collection (gopsutil → go-metrics)

metrics/
  ├── command.go           CLI entry + HTTP handlers + Agent loop
  ├── options.go           option structs
  └── config.go            config loading
```

### Verified

- `go build ./...` compiles
- `go vet ./core/metrics/... ./metrics/...` passes
- `go test ./core/metrics/...` — 9 tests passed (2 packages)
- ~1000 lines of code total