# Keyring 凭据存储模块计划

> **状态：📋 已计划，暂缓实施（paused）。** 当前 API key 仍以明文存于配置文件。

## 背景

目前 `ask`、`git commit/review`、`budget` 的 API key 均以明文存在 `~/.config/mu/*-config.json`。
目标：提供 `mu secret` 子命令，将密钥存入 OS 原生 keyring，并让各模块在解析 API key 时
优先从 keyring 读取（`--flag/env → config 文件 → keyring`），配置文件可不再落盘明文。

## 依赖

`github.com/zalando/go-keyring`（纯 Go，无 CGO，保持跨平台 `CGO_ENABLED=0` 编译）

| 平台 | 后端 |
|------|------|
| macOS | Keychain（`/usr/bin/security`） |
| Linux/\*BSD | Secret Service 协议（D-Bus，GNOME Keyring/KWallet） |
| Windows | Credential Manager（target = `service:user`） |

## 存储模型

go-keyring 是二维 key/value：`(service, user) → value`。

```go
keyring.Set(service string, user string, password string) error
keyring.Get(service string, user string) (string, error)
keyring.Delete(service string, user string) error
```

## 目录结构

```
internal/secret/
  ├── options.go     set / get / list / rm 子命令 flag
  ├── command.go     CLI 入口，逻辑委托 internal/core/secret
  └── set.go         可选：config.Register(&secretSetter{})（若并入 mu set）

internal/core/secret/
  ├── keyring.go     Set/Get/Delete 封装（含错误映射、Linux 无头降级提示）
  └── index.go       list 索引（若采纳索引方案）
```

## CLI 接口

```
mu secret set <key> <value>      # 例：mu secret set ask.api_key sk-xxx / budget.deepseek sk
mu secret get <key>              # 输出明文（可加 --quiet）
mu secret list                   # 列出已存 key 名
mu secret rm <key>               # 删除
mu secret rm --all               # 清空
```

`<key>` 命名约定：`<module>.<field>`（`ask.api_key`、`git.default`、`budget.deepseek`、
`budget.openrouter`、`es.password`…），命令层负责 `key → (service,user)` 映射，底层落 OS keyring。

## 集成：解析链

统一优先级：**`--flag` / env → config 文件 → keyring**（config 为空串时继续查 keyring）。

| 模块 | 当前解析点 | 改动 |
|------|-----------|------|
| `ask` | `internal/ask/command.go:155`（`cfg.APIKey == ""` 报错） | 为空时查 keyring |
| `git commit/review` | `internal/git/review.go:71`、`commit.go:66`（provider.APIKey） | 为空时查 keyring |
| `budget` | `internal/core/budget/config.go:60` `ResolveAPIKey` | 为空时查 keyring |

## 错误处理（Linux 无头环境）

- D-Bus Secret Service 不可用时 `keyring.Get` 返回错误 → 降级：回退 config 文件 + 打印提示
  （提示安装 `libsecret-tools` / `gnome-keyring` / 解锁 keyring）
- `mu secret set` 在 keyring 不可用时给出可操作错误信息

## 测试

- `internal/core/secret` 单测用 `keyring.MockInit()`（go-keyring 提供内存实现）
- 覆盖：Set/Get/Delete round-trip、key 命名映射、keyring 不可用降级路径

## 实施步骤

| 步骤 | 内容 |
|------|------|
| 1 | `go get github.com/zalando/go-keyring` + 验证无 CGO |
| 2 | `internal/core/secret/keyring.go` + `index.go` |
| 3 | `mu secret` CLI（options.go/command.go）注册到 `cmd/mu/myutilities.go` |
| 4 | 单测（MockInit） |
| 5 | 集成 ask / git / budget 解析链 |
| 6 | 文档：README、AGENTS.md（config 约定加 keyring 说明）、ROADMAP #6 → Done |
| 7 | 全项目 build/vet/test 验证 |

## 待决策项

### D1. `(service, user)` 键命名

- **方案 A（推荐）— 模块分离：** service = `mu-ask` / `mu-budget` / `mu-git`，user = `api_key` 或 provider 名。
  钥匙串 GUI（Seahorse/Keychain Access）中每个模块显示为独立条目，更易辨识与管理。
- **方案 B — 单一 service：** service = `mu`，user = `<module>.<field>`。更紧凑，但 GUI 中所有条目
  混在同一 service 下，删除/排查需靠 user 名区分。

### D2. `mu secret list` 的实现（go-keyring 不支持枚举条目）

- **方案 1（推荐）— 维护索引文件：** 在 `~/.config/mu/secret-index.json` 记录 `key → (service,user)`
  映射，`set`/`rm` 时同步增删。`list` 直接读索引。缺点：索引与 OS keyring 可能不一致（用户手动
  用 `secret-tool` 增删时）；需处理索引损坏/缺失。
- **方案 2 — 不维护索引：** `list` 依赖平台工具（macOS `security dump-keychain` 过滤 / Linux
  `secret-tool search`），或直接不支持 list。实现更简单，但跨平台输出格式不统一、解析脆弱。
- **方案 3 — 仅支持已知键：** `list` 输出预设的已知 key 集合（配置文件里定义的可选 key 全集），
  标注哪些已存在。无需索引，但不反映用户自定义键。

### D3. 是否并入 `mu set`

- **方案 A（推荐）— 独立 `mu secret` 命令：** 与 ROADMAP 原计划一致，职责清晰。
- **方案 B — 并入 `mu set`：** 复用 `ModuleSetter` 注册机制，但语义（存 keyring 而非 config 文件）
  与现有 setter 不同，侵入较大。
