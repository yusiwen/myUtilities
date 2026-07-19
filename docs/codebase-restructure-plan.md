# Codebase Restructure Plan

本文档记录了 myUtilities 项目代码组织与结构优化（对应 `docs/tasks.md` 第 6-11 项）的详细计划。

## 现状问题清单

| # | 问题 | 严重程度 | 说明 |
|---|------|---------|------|
| A | 包名冲突 | 🔴 高 | `core/crypto` vs `crypto/`、`core/git` vs `git/`、`core/runner` vs `runner/`、`core/proxy` vs `proxy/`，4 组同名包，必须用 import alias |
| B | 版本在 main 包 | 🟡 中 | `Version` / `CommitSHA` / `BuildTime` 在 `package main`，其他包无法直接引用 |
| C | PascalCase 文件名 | 🟡 中 | `core/proxy/Proxy.go` 等 6 个文件违反 Go 文件名惯例 |
| D | 项目根目录杂乱 | 🟡 中 | `main.go` `myutilities.go` `version.go` 3 个入口文件在根目录，未按标准布局组织 |
| E | CLI 命令名 ≠ 包名 | 🟢 低 | `name:"install"` → `package installer`，`name:"run"` → `package runner`，`name:"jar"` → `package jarinfo` |
| F | core/net 文件分裂 | 🟢 低 | `interface.go` vs `interfaces.go`，内容边界模糊 |
| G | 仅有 2 处 TODO | 🟢 低 | `installer/command.go` 中 deb/rpm 和 powershell 两个 TODO |
| H | 0 处注释掉的代码 | 🟢 低 | 实际已经较干净，无需专门清理 |

## Phase 1 — Quick Wins

预计总工时：**~45分钟**。三项相互独立，可并行执行。风险低。

### 1-① 版本信息抽到独立包

**目标：** 解决 B，让所有包都能直接引用版本信息。

**操作步骤：**

1. 新建 `core/version/version.go`:
   ```go
   package version

   var (
       Version   = "unknown version"
       BuildTime = "unknown time"
       CommitSHA = ""
   )
   ```

2. 删除根目录 `version.go`（原 `package main`）

3. 修改 `main.go`:
   - 添加 import `v "github.com/yusiwen/myUtilities/core/version"`
   - 所有 `Version` / `CommitSHA` / `BuildTime` 改为 `v.Version` / `v.CommitSHA` / `v.BuildTime`

4. 修改 `Makefile`:
   - `main.Version` → `github.com/yusiwen/myUtilities/core/version.Version`
   - `main.CommitSHA` → `github.com/yusiwen/myUtilities/core/version.CommitSHA`
   - `main.BuildTime` → `github.com/yusiwen/myUtilities/core/version.BuildTime`

5. 验证：`make build && ./bin/mu --version`

**涉及文件：** `core/version/version.go`（新建）、`main.go`（修改）、`Makefile`（修改）

---

### 1-② 修复 PascalCase 文件名

**目标：** 解决 C，将 6 个文件名改为 Go 惯例的 snake_case。

**操作步骤：**

纯 `git mv`，不需要改文件内 `package` 声明（Go 按目录名分组）。

| 当前路径 | 改为 |
|----------|------|
| `core/proxy/Proxy.go` | `core/proxy/proxy.go` |
| `core/proxy/db/DBProxy.go` | `core/proxy/db/dbproxy.go` |
| `core/watcher/GitWatcher.go` | `core/watcher/gitwatcher.go` |
| `core/watcher/FileWatcher.go` | `core/watcher/filewatcher.go` |
| `core/runner/CommandRunner.go` | `core/runner/commandrunner.go` |
| `mock/oauth/AuthServer.go` | `mock/oauth/authserver.go` |

**需检查：** 其他文件是否 import 了这些包（Go 按包名 import，不依赖文件名，所以一般不需要改 import）。但需要确认没有文件用 `import "./..."` 相对路径。

**涉及文件：** 6 个文件重命名

---

### 1-③ 清理 TODO

**目标：** 解决 G，处理 `installer/command.go` 中的 2 个 TODO。

**操作步骤：**

1. 在 GitHub 上为每个功能创建 issue（如果还没有）
2. 将 TODO 替换为指向 issue 的标准注释:
   ```go
   // TODO(#123): deb,rpm etc
   // TODO(#124): powershell
   ```

或者如果决定短期内不做，直接删除 TODO 行。

**涉及文件：** `installer/command.go`（修改）

---

## Phase 2 — 包名冲突

预计总工时：**~2小时**。风险中等，需要逐个验证编译通过。

### 2-④ 解决 4 组包名冲突

**目标：** 解决 A，消除所有 import alias 需求。

**背景：** 4 组同名包：

| 包名 | 命令路径 | core 路径 | 当前 alias 方式 |
|------|---------|-----------|----------------|
| `git` | `git/` | `core/git/` | `coregit "..."` |
| `runner` | `runner/` | `core/runner/` | `corerunner "..."` |
| `proxy` | `proxy/` | `core/proxy/` | (无显式 alias，通过调用链间接使用) |
| `crypto` | `crypto/` | `core/crypto/` | `corecrypto "..."` |

#### 选项 A（推荐）：核心包重命名

在 `core/` 侧改名，不影响 CLI 用户接口：

| 当前 | 改为 |
|------|------|
| `core/git` → `package gitcore` | 目录 `core/git/` → `core/gitcore/` |
| `core/runner` → `package cmdexec` | 目录 `core/runner/` → `core/cmdexec/` |
| `core/proxy` → `package proxycore` | 目录 `core/proxy/` → `core/proxycore/` |
| `core/crypto` → 保持 `package crypto` | 目录不变（由上层用 alias 区分） |

受影响的 import 更新：

| 旧 import | 新 import | 涉及文件 |
|-----------|----------|---------|
| `core/git` | `core/gitcore` | `git/commit.go`、`git/ignore.go` |
| `core/runner` | `core/cmdexec` | `runner/runner.go`、`runner/options.go` |
| `core/proxy` | `core/proxycore` | `proxy/dbproxy.go`、`core/proxy/db/DBProxy.go`（`core/proxy` 本身需改名） |

选型理由：
- ✅ 不改变 CLI 命令名（用户接口不变）
- ✅ 不改变文档中的命令示例
- ✅ 不改变 Makefile 构建目标
- ✅ 不改变前端 embed 路径
- ❌ 需要更新 6 个文件的 import

#### 选项 B（备选）：命令包重命名

| 当前 | 改为 | CLI 命令名 |
|------|------|-----------|
| `package git` in `git/` | `package gittool` in `gittool/` | `name:"git"` 不变 |
| `package runner` in `runner/` | `package runcmd` in `runcmd/` | `name:"run"` 不变 |
| `package proxy` in `proxy/` | `package proxysrv` in `proxysrv/` | `name:"proxy"` 不变 |
| `package crypto` in `crypto/` | `package cryptotool` in `cryptotool/` | `name:"crypto"` 不变 |

选型理由：
- ✅ core 保持纯洁，不改名
- ❌ Makefile 路径要变（`go build ./crypto/` → `./cryptotool/`）
- ❌ 前端 embed 相对路径要变（`crypto/frontend/dist` → `cryptotool/frontend/dist`）
- ❌ 目录名与 CLI 命令名不一致，增加困惑

**结论：推荐选项 A。**

---

## Phase 3 — 标准项目布局

预计总工时：**~1天**。风险高，需要谨慎评估。

### 3-⑤ 迁移到 cmd/ + internal/

**目标：** 解决 D，将项目调整为标准 Go 项目布局。

**目标布局：**

```
myUtilities/                     (go.mod)
├── cmd/
│   └── mu/
│       ├── main.go              (package main, 仅入口+ kong.Parse)
│       └── myutilities.go       (package main, CLI 结构体)
├── internal/                    (外部不可导入)
│   ├── gateway/  wol/  es/  ...
│   ├── crypto/  diff/  k8s/  ...
│   └── core/                    (internal/core/)
│       ├── crypto/  gitcore/  cmdrunner/  proxycore/  ...
│       └── net/  openai/  store/  watcher/
├── pkg/                          (外部可导入)
│   └── version/                  (从 core/version/ 移出，供外部引用)
├── web/                          (前端资源)
│   └── shared/frontend/
├── Makefile
└── README.md
```

**影响评估：**

| 方面 | 影响 |
|------|------|
| 文件移动 | ~50+ 个 Go 文件，~10 个 frontend 目录 |
| import 路径 | 所有 `github.com/yusiwen/myUtilities/gateway` → `github.com/yusiwen/myUtilities/internal/gateway` |
| `//go:embed` | 不改变（路径相对于文件自身，文件跟着目录走） |
| Makefile | `go build -o bin/mu .` → `go build -o bin/mu ./cmd/mu/` |
| ldflags | 不改变（版本包路径已在 Phase 1 固定）|
| `install.sh` | 不改变（只引用二进制） |
| CLI 命令 | 不改变 |
| Web UI 路由 | 不改变 |
| 用户可见行为 | 无变化 |

**决策分析：** 此项目是 CLI 工具 + Web UI，不会作为库被外部导入。`internal/` 的保护价值有限。`cmd/mu/` 的好处是入口清晰，但代价是约 50 个文件的 import 路径变更和回归测试成本。

**建议：** 等 Phase 1 + 2 完成后评估是否仍需要做 Phase 3。如果 Phase 2（解决包名冲突）已经大幅改善代码结构，Phase 3 可酌情降级或跳过。

---

## Phase 4 — 命名收尾

预计总工时：**~1小时**。风险低。

### 4-⑥ 统一命名规范

**目标：** 解决 E + F + H，补完剩余的命名和注释问题。

**操作步骤：**

1. **修复 CLI 命令名 vs 包名不一致**
   这些是 Kong 的 `name:` tag 决定的，包名可以保持内部命名，不需要对齐 CLI 名。但可以考虑加注释说明。

2. **合并或重划分 `core/net/`**
   `interface.go` 和 `interfaces.go` 可以合并，或者重命名为 `netiface.go` + `wol.go` 等更明确的名称。

3. **统一注释语言为英文**
   清理 `core/proxy/Proxy.go`、`core/proxy/db/DBProxy.go`、`core/watcher/FileWatcher.go`、`mock/oauth/AuthServer.go` 中的中文注释，改为英文。

---

## 工作量汇总

| Phase | 内容 | 涉及文件数 | 预计工时 | 风险 |
|-------|------|-----------|---------|------|
| 1-① | 版本独立包 | 3-4 | ~30min | 🟢 低 |
| 1-② | 修复文件名 | 6 | ~10min | 🟢 低 |
| 1-③ | 清理 TODO | 1 | ~5min | 🟢 低 |
| 2-④ | 包名冲突 | 6-8 | ~2h | 🟡 中 |
| 3-⑤ | 标准布局 | ~50+ | ~1d | 🔴 高 |
| 4-⑥ | 命名收尾 | 5-10 | ~1h | 🟢 低 |

## 建议执行顺序

1. **Phase 1** — Quick Wins，立即可做，风险低
2. **Phase 2** — 包名冲突，解决最大痛点
3. **(讨论) Phase 3** — 标准布局，评估 ROI 后决定
4. **Phase 4** — 命名收尾，最后补完
