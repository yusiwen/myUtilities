# WireGuard 配置管理模块计划 (`mu wg`)

> **状态：📋 已计划，待实施。** 设计已与用户确认，可按本计划实施。

## 背景

需要一个管理 WireGuard 配置文件的小模块：

1. 读取当前配置文件，默认 `/etc/wireguard/wg0.conf`，可用参数指定。
2. 列出当前所有对端（peer）信息。
3. 维护「对端 ↔ 名字/备注」的对应关系。这个名字/备注是官方配置文件里不支持的，需要额外管理。

### 核心难点

WireGuard 官方配置（wg-quick 格式）只有 `PublicKey`、`Endpoint`、`AllowedIPs`、
`PersistentKeepalive`、`PresharedKey` 等字段，**没有稳定的「名字」字段**。
因此对端元数据必须外置存储，且**以 `PublicKey` 作为唯一键**（IP 会变，key 不会变）。
另外官方工具（`wg setconf` / `wg syncconf`）重新序列化配置时会剥掉注释，
所以「把名字写进 conf 注释」的方案不可靠。

### 已确认的取舍（用户拍板）

| 决策点 | 选择 |
|--------|------|
| 元数据存储位置 | 旁车 JSON（`<conf>.meta.json` 放在 conf 旁边） |
| 功能范围 | 只做「元数据管理 + 列表」，不读写 conf 文件本身 |
| Web UI | 纯 CLI，无 serve/前端 |
| 实时状态 | 可选 `--live`（调 `wg show`，需 root） |

## 存储模型

### 旁车元数据文件

路径：`<conf 路径>.meta.json`，例如 `/etc/wireguard/wg0.conf.meta.json`。

```json
{
  "version": 1,
  "peers": {
    "BASE64PUBLICKEY1=": { "name": "home-nas", "note": "家庭 NAS，通过 NAT 出口" },
    "BASE64PUBLICKEY2=": { "name": "phone",   "note": "" }
  }
}
```

- 键为 peer 的 `PublicKey`（44 位 base64，唯一且稳定）。
- 文件权限 `0600`。
- 优点：元数据跟着配置走，备份/迁移天然一致；无全局命名冲突；写权限要求与改 conf
  相同（都是 root），无权限落差。

### conf 解析

手写轻量 INI 解析器，**只读、绝不回写** conf 文件。特性：

- key 大小写不敏感（WireGuard 解析器不区分大小写）。
- `#` 注释整行跳过；值按第一个 `=` 分割。
- 按 `[Interface]` / `[Peer]` 分段，保留段落顺序。
- `AllowedIPs` 为逗号分隔的多个地址。

## 目录结构

```
internal/wg/            # CLI 封装（kong 子命令）
  ├── options.go        # Options: list / rename / note / prune 子命令 flag
  ├── command.go        # list 实现 + resolveConfPath()（--config/--interface/env/默认）
  └── meta.go           # rename / note / prune 实现 + peer 查找（名字精确 / pubkey 前缀）

internal/core/wg/       # 业务逻辑，可独立测试
  ├── conf.go           # Parse(data) → *Config{Interface, []Peer}
  ├── meta.go           # MetaStore: LoadMeta / SaveMeta（0600）
  ├── join.go           # ListPeers(conf, meta) → []PeerRow（join + 标记未命名/orphan）
  └── live.go           # ShowLive(iface) → 解析 `wg show <iface> dump`，按 pubkey 合并
```

注册：`cmd/mu/myutilities.go` 加 `Wg wg.Options cmd:"" name:"wg" help:"WireGuard config management."`

## CLI 接口

```
mu wg list    [--config PATH] [--interface wg0] [--live] [--json]
mu wg rename  [--config PATH] <peer> <新名字>
mu wg note    [--config PATH] <peer> <备注>
mu wg prune   [--config PATH]        # 清理 meta 中已从 conf 消失的 pubkey
```

### conf 路径解析优先级

`--config <path>` > `--interface <name>`（推导 `/etc/wireguard/<name>.conf`）>
env `MU_WG_CONFIG` > 默认 `/etc/wireguard/wg0.conf`。

### `<peer>` 查找规则

1. 名字精确匹配（meta 中的 `name`）→ 直接命中。
2. 否则按 pubkey 前缀匹配（≥6 位），唯一命中才采用。
3. 无法唯一确定时列出候选（相近名字/前缀），而非报错。

### list 输出

tabwriter 列：`NAME | PUBLIC KEY(短) | ENDPOINT | ALLOWED IPS | KEEPALIVE | NOTE`。
`--live` 追加 `HANDSHAKE | RX | TX`。`--json` 输出结构化数据供脚本使用。

## 边界处理

- 未命名的 peer 显示 pubkey 前 8 位。
- meta 中有、conf 中已消失的 pubkey（orphan）→ list 时警告，`prune` 清理。
- peer 找不到时列出候选。
- `--live` 失败（未安装 `wg` / 非 root / 接口不存在）→ 非致命警告，降级为纯 conf 输出。
- 不写回 conf 文件本身（v1 范围确认）。

## 测试

- `internal/core/wg/conf_test.go` — 解析：多 peer、大小写、注释、空值、AllowedIPs 逗号分隔。
- `internal/core/wg/meta_test.go` — Load/Save round-trip、损坏文件容错。
- `internal/core/wg/live_test.go` — `wg show <iface> dump` 输出解析。
- `internal/core/wg/join_test.go` — 未命名 / orphan / 正常 join。

## 实施步骤

| 步骤 | 内容 |
|------|------|
| 1 | `internal/core/wg/conf.go` + `conf_test.go` |
| 2 | `internal/core/wg/meta.go` + `meta_test.go` |
| 3 | `internal/core/wg/join.go` + `join_test.go` |
| 4 | `internal/core/wg/live.go` + `live_test.go` |
| 5 | `internal/wg/options.go` / `command.go` / `meta.go`（CLI） |
| 6 | 注册 `cmd/mu/myutilities.go`，README.md 命令清单、docs/wg.md、CODEBASE.md |
| 7 | 全项目 build / vet / fmt / test 验证 |

## 待决策项（后续可选）

### D1. 是否并入 `mu set wg`

- **方案 A（推荐）— 不并入：** conf 路径用 flag + env 足够，保持模块最小。
- **方案 B — 并入：** `config.Register(&wgSetter{})` 提供 `mu set wg --default-config`，
  持久化默认 conf 路径。符合项目 `<module>-config.json` 惯例，但 v1 用不到。

### D2. 是否支持 conf 增删改

- **方案 A（推荐）— 不做：** 只读 conf，避免安全重写（保留注释/格式）的复杂度和风险。
- **方案 B — 增加 add/remove/set：** 需要安全重写器，风险高，留待后续版本。
