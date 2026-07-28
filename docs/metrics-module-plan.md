# mu metrics 模块完整计划

## 总体架构

```
mu metrics
  ├── serve    HTTP server（接收 Agent 上报 + 查询 API + 前端）
  ├── agent    采集 daemon（按间隔抓取并写入本地 bbolt 或推送到远端 server）
  └── compact  手动压缩 / 清理过期数据
```

### 两种角色

- **`mu metrics serve`** — 启动 HTTP server，可选 `--agent` 参数同时启动本地采集（嵌入模式，单机用）
- **`mu metrics agent`** — 采集本机指标，有 `--server` 则推送到远端 server，否则写入本地 bbolt

## 目录结构

```
core/metrics/
  ├── model.go           Metric / DataPoint / WriteRequest 类型
  ├── tsdb.go            bbolt 写入 / 查询 / 压缩
  ├── tsdb_test.go
  └── collector/
      ├── collector.go   Collector 接口 + go-metrics Registry 集成
      └── os.go          gopsutil → Gauge 更新

metrics/
  ├── command.go         mu metrics serve / agent 入口
  ├── options.go         参数结构体
  └── config.go          metrics-config.json 加载
```

---

## 1. 核心数据模型 — `core/metrics/model.go`

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
    Timestamp int64             `json:"time,omitempty"` // UnixNano，为空时 server 端取当前时间
    Value     float64           `json:"value"`
}
```

---

## 2. TSDB 引擎 — `core/metrics/tsdb.go`

使用 `go.etcd.io/bbolt`（已从 `coreos/bbolt` 迁移完成），数据库文件独立。

### bbolt Key 设计

```
Bucket: "series"
  Key: <metric_name>\x00<fnv64a(sorted_tags)>\x00<timestamp_be(8字节)>
  Value: <float64bits(value)(8字节)>
```

- `fnv64a(sorted_tags)` = FNV-1a 64bit hash of sorted `k1=v1,k2=v2` → 相同的 metric+tags 聚簇存储
- `timestamp_be` = big-endian int64 UnixNano → 有序，支持 `Cursor.Seek()` 范围扫描
- 单条记录 ~40 字节，百万级数据量对 bbolt 无压力

### 接口

```go
type DB struct { db *bolt.DB }

func Open(path string) (*DB, error)

// Write 写入单个数据点
func (db *DB) Write(name string, tags map[string]string, ts time.Time, value float64) error

// WriteBatch 批量写入（agent 每次 flush 用）
func (db *DB) WriteBatch(metrics []Metric) error

// Query 按 metric + tags + 时间范围查询
func (db *DB) Query(name string, tags map[string]string, from, to time.Time, limit int) ([]Metric, error)

// ListMetrics 返回所有 metric 名称
func (db *DB) ListMetrics() ([]string, error)

// Compact 删除 cutoff 之前的数据点（retention<=0 时跳过）
func (db *DB) Compact(retention time.Duration) error
```

### Compact 实现策略

```go
func (db *DB) Compact(retention time.Duration) error {
    if retention <= 0 {
        return nil  // 永久保留
    }
    cutoff := time.Now().Add(-retention)
    cutoffBE := uint64ToBE(uint64(cutoff.UnixNano()))

    return db.Update(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("series"))
        c := b.Cursor()
        for k, _ := c.First(); k != nil; k, _ = c.Next() {
            // Key 格式: <name>\x00<hash>\x00<timestamp_be(8字节)>
            // timestamp_be 是最后 8 字节
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

### Collector 接口

```go
type Collector interface {
    Name() string
    Collect(r metrics.Registry) error
}
```

**go-metrics Registry 的角色**：Agent 进程内持有的一棵当前值树，每次采集 tick 更新所有 Gauge，然后 flush 到 bbolt。

### OS 指标采集清单 — `os.go`

依赖 `github.com/shirou/gopsutil/v4`。

| Metric | Tags | 来源 |
|--------|------|------|
| `cpu.used.percent` | — | gopsutil `cpu.Percent(0, false)` |
| `cpu.per_cpu.percent` | `cpu=N` | gopsutil `cpu.Percent(0, true)` |
| `memory.used.percent` | — | gopsutil `mem.VirtualMemory().UsedPercent` |
| `memory.used.bytes` | — | 同上 `.Used` |
| `disk.used.percent` | `mount=/`, `device=sda1` | gopsutil `disk.Usage()` |
| `disk.io.bytes` | `device=sda`, `direction=read/write` | gopsutil `disk.IOCounters()` |
| `net.io.bytes` | `interface=eth0`, `direction=in/out` | gopsutil `net.IOCounters()` |
| `load.1m` / `load.5m` / `load.15m` | — | gopsutil `load.Avg()` |

采集时调用 gopsutil 获取值，然后更新 go-metrics Registry 中对应 `Gauge` / `GaugeFloat64`。

---

## 4. CLI 命令 — `metrics/`

### 参数定义 — `options.go`

```go
type ServeOptions struct {
    Port      int    `help:"HTTP API port." default:"8096"`
    Retention string `help:"Data retention (e.g. 30d, 7d, 0=forever)." default:"0"`
    Agent     bool   `help:"Also run agent locally."`
    Interval  string `help:"Collect interval (only with --agent)." default:"30s"`
}

type AgentOptions struct {
    Server   string `help:"Metrics server URL to report to." default:""`
    Interval string `help:"Collect interval." default:"30s"`
    Hostname string `help:"Override hostname for tags." default:""`
    Retention string `help:"Local data retention (when no server)." default:"0"`
}
```

### 配置示例 — `metrics-config.json`

```json
{
  "retention": "30d",
  "collect_interval": "30s",
  "hostname": "Prod-Web-01",
  "server_url": "http://metrics-server:8096"
}
```

- `"retention"` 支持格式：`"30d"`、`"7d"`、`"24h"`、`"90d"`。`"0"` 或不设置 = 永久保留。
- `"hostname"` 可选，默认 `os.Hostname()`。Agent 会在每次写入时自动注入 `host` tag。
- `server_url` 仅 agent 使用，为空时写入本地 bbolt。

### 主入口 — `command.go`

```go
type MetricsCmd struct {
    Serve ServeOptions `cmd:"" help:"Start metrics HTTP server."`
    Agent AgentOptions `cmd:"" help:"Start metrics collection agent."`
}
```

注册到 `myutilities.go`：
```go
Metrics MetricsCmd `cmd:"" name:"metrics" help:"Time-series metrics collection and querying."`
```

### 具体行为

```bash
# 启动 server（HTTP API，接收 agent 上报 + 查询 + compact）
mu metrics serve --port 8096 --retention 30d

# 启动 agent（采集本机指标 → 上报 server）
mu metrics agent --server http://metrics-server:8096 --interval 30s

# 启动 agent+server 合一（嵌入模式，单机用）
mu metrics serve --agent --interval 30s --retention 30d

# 手动触发 compaction（不启动 agent/server）
mu metrics compact --retention 30d
```

---

## 5. HTTP API — `serve` 模式

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/metrics` | `GET` | 列出所有 metric 名称 |
| `/api/metrics/:name` | `GET` | 查询数据点 |
| `/api/metrics/write` | `POST` | Agent 上报数据 |

### 查询 `GET /api/metrics/:name`

参数：

| 参数 | 说明 | 示例 |
|------|------|------|
| `from` | 起始时间（RFC3339） | `2024-01-01T00:00:00Z` |
| `to` | 结束时间 | `2024-01-02T00:00:00Z` |
| `tags` | URL encoded `k=v,k=v` | `host%3DHostA,cpu%3D0` |
| `limit` | 最大返回点数 | `1000` |

响应：

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

### 列出所有 metrics `GET /api/metrics`

```json
["cpu.used.percent", "memory.used.bytes", "disk.used.percent"]
```

### Agent 上报 `POST /api/metrics/write`

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

- `time` 可选，为空时 server 取当前时间
- `host` tag 由 Agent 自动注入，默认为 `os.Hostname()`，可通过 `--hostname` 或配置文件覆盖

---

## 6. Agent 采集循环

```go
func (a *Agent) Run(ctx context.Context) error {
    registry := metrics.NewRegistry()  // go-metrics
    collectors := []Collector{
        collector.NewOSCollector(),
    }

    interval, _ := time.ParseDuration(a.cfg.Interval)
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    // 首次采集
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

    // 1. 采集 → 更新 go-metrics Gauges
    for _, c := range collectors {
        c.Collect(r)
    }

    // 2. flush 到 bbolt 或远端 server
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

### Server 推送失败处理

当 agent 配置了 `server_url` 推送到远端 server 时，如果 server 不可达，采用**指数退避重试 + 本地缓存**策略：

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
            return // 推送成功
        }
        log.Printf("Push to server failed (attempt %d/%d): %v", attempt+1, maxRetries, err)
    }

    // 最终失败 → 写入本地 bbolt 缓存，避免数据丢失
    log.Printf("Server unreachable, caching locally")
    if a.cfg.tsdb != nil {
        if err := a.cfg.tsdb.WriteBatch(batch); err != nil {
            log.Printf("Local cache write error: %v", err)
        }
    }
}
```

**行为：**

| 场景 | 结果 |
|------|------|
| Server 正常 | 推送到 server，不写本地 |
| Server 临时故障 | 退避重试 3 次（1s → 2s → 4s） |
| Server 长期不可达 | 回退写入本地 bbolt，数据不丢 |
| Server 恢复后 agent 下次推送 | 恢复 online 模式，直接推 server |
| 无 server（纯本地模式） | 每次直接写入本地 bbolt |

### Agent 自动注入 `host` tag

- 默认值：`os.Hostname()` 自动获取
- 可覆盖：`--hostname my-custom-name` 或 `metrics-config.json` 中的 `hostname` 字段
- 写入时 agent 在每一条数据的 tags 中注入 `host:<hostname>`

### 查询时区分主机

```bash
# HostA 的所有 CPU 指标
GET /api/metrics/cpu.used.percent?tags=host%3DHostA

# 所有主机的 CPU 指标
GET /api/metrics/cpu.used.percent

# HostA 的 0 号 CPU
GET /api/metrics/cpu.used.percent?tags=host%3DHostA,cpu%3D0
```

bbolt key 中的 `fnv64a(sorted_tags)` 因为包含 `host`，不同主机的数据自动进入不同的 key 空间，查询时按 `metric_name + tags_hash` 前缀扫描即可区分。

---

## 7. Compaction（数据过期）

### 配置

```json
{
  "retention": "30d"
}
```

- `"0"`、`""`、不设置 → 永久保存
- `"30d"` → 保留 30 天
- `"7d"` → 保留 7 天
- `"24h"` → 保留 24 小时

### 自动触发

Agent 或 Server 启动时执行一次 `Compact(retention)`。之后每小时自动执行一次。`Compact(0)` 不做任何操作（永久保存）。

### 手动触发

```bash
mu metrics compact --retention 30d
```

---

## 8. 数据文件路径

- **数据库文件**：`~/.local/share/mu/metrics/metrics.db`
- **配置文件**：`~/.config/mu/metrics-config.json`

---

## 9. 新增依赖

```
go.etcd.io/bbolt v1.3.11         （已完成迁移）
github.com/rcrowley/go-metrics    → 内存指标 Registry
github.com/shirou/gopsutil/v4     → OS 指标采集
```

---

## 10. 实现顺序

| 阶段 | 内容 | 预计改动量 |
|------|------|-----------|
| 1 | `core/metrics/model.go` + `tsdb.go`（Write/Query/Compact/ListMetrics） | ~300 行 |
| 2 | `core/metrics/collector/`（collector.go + os.go，go-metrics 集成） | ~200 行 |
| 3 | `metrics/options.go` + `config.go` + `command.go`（serve 子命令 + HTTP API） | ~250 行 |
| 4 | `metrics/command.go`（agent 子命令 + 采集循环 + flush + compact） | ~200 行 |

---

## 11. 未来可扩展（当前版本不做）

| 功能 | 时机 |
|------|------|
| Docker 容器指标采集（cgroup 或 Docker SDK） | 第二版 |
| Agent token 认证（`--token` + server 校验 + 自动绑定 host） | 安全需求时 |
| Web 前端图表（gateway 集成 `/metrics/`） | 集中展示时 |
| 聚合查询（`avg`/`max`/`min`/`sum` + `window` 时间窗口） | 查询需求明确时 |
| Prometheus remote write 兼容 | 需要接入 Grafana 时 |

---

## 12. 名词约定

| 术语 | 含义 |
|------|------|
| **metric** | 指标名称，如 `cpu.used.percent` |
| **tags** | 标签键值对，用于区分同一指标的不同维度，如 `{host: HostA, cpu: 0}` |
| **data point** | 一个时间戳 + 值的组合 |
| **agent** | 采集端，运行在被监控机器上 |
| **server** | 存储 + 查询端，接收 agent 上报的数据 |
| **retention** | 数据保留时长 |
| **compaction** | 删除过期数据的操作 |

---

## 13. 实现进度

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | `core/metrics/model.go` + `tsdb.go`（Write/Query/Compact/ListMetrics） | ✅ |
| 2 | `core/metrics/collector/`（collector.go + os.go，go-metrics 集成） | ✅ |
| 3 | `metrics/options.go` + `config.go` + `command.go`（serve 子命令 + HTTP API） | ✅ |
| 4 | 注册 `metrics` 命令到 `myutilities.go`，全项目编译通过 | ✅ |
| 5 | `core/metrics/tsdb_test.go`（9 个测试全部通过） | ✅ |
| 6 | `metrics/options.go` 新增 `QueryOptions` + `query` 子命令 | ✅ |
| 7 | `metrics/command.go` 新增 `query.Run()`（table/json/csv 三格式输出） | ✅ |
| 8 | Agent 推送重试 + 本地缓存策略实现 | ✅ |
| 9 | Debug 日志（`--debug` flag + `debug_log` 配置 + 关键路径日志） | ✅ |
| 10 | `query` 和 `compact` 改为通过 HTTP API 与 server 交互（不再读本地 bbolt） | ✅ |

### 已创建的文件清单

```
core/metrics/
  ├── model.go             数据类型定义
  ├── tsdb.go              核心 TSDB 引擎
  ├── tsdb_test.go         9 个测试用例
  └── collector/
      ├── collector.go     Collector 接口
      └── os.go            OS 指标采集（gopsutil → go-metrics）

metrics/
  ├── command.go           CLI 入口 + HTTP handlers + Agent 循环
  ├── options.go           参数结构体
  └── config.go            配置加载
```

### 已验证

- `go build ./...` 编译通过
- `go vet ./core/metrics/... ./metrics/...` 通过
- `go test ./core/metrics/...` — 9 tests passed（2 packages）
- 累计代码 ~1000 行
