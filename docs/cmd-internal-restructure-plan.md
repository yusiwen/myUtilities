# cmd/ + internal/ 布局重构计划

> 状态：待执行
> 关联：`docs/codebase-restructure-plan.md` Phase 3-⑤（本计划为其细化执行版）
> 目标：将 myUtilities 调整为标准 Go 项目布局（cmd/ + internal/），编译为单一 standalone executable。

## 决策记录

| 决策点 | 结论 |
|--------|------|
| core/ 业务逻辑包位置 | `internal/core/`（纯 CLI 工具，所有包对外不可导入） |
| 是否顺带解决包名冲突 | **仅做目录迁移**，4 组同名包 alias（`corecrypto`/`coregit`/`corerunner` 等）保留 |
| shared/frontend/ 位置 | 移到 `web/shared/frontend/` |
| commit 拆分 | 3 个 commit：①移动+import ②构建系统 ③文档 |
| Makefile `default:` 无版本注入 | **保持现状**（只把 `.` 改为 `./cmd/mu`），不顺手改成复用 GOBUILD；此为既有行为，本次不改变 |

## 目标布局

```
myUtilities/
├── cmd/mu/                     # 唯一可执行入口 (package main)
│   ├── main.go                 # kong.Parse + runSet
│   ├── myutilities.go          # MyUtilities CLI 结构体
│   └── version.go              # Version/CommitSHA/BuildTime (ldflags -X main.* 不变)
├── internal/                   # 所有业务包（外部不可导入）
│   ├── ask/ budget/ completion/ crypto/ diff/ es/ gateway/ git/
│   ├── installer/ jarinfo/ k8s/ metrics/ misc/ mock/ network/
│   ├── proxy/ qrcode/ runner/ scip/ serve/ svcreg/ watch/ wol/
│   ├── mock/oauth/  installer/templates/          # 嵌套子包
│   └── core/                   # internal/core/（原 core/ 整棵移入）
├── web/shared/frontend/        # 共享 partials（原 shared/frontend/）
├── Makefile  go.mod  go.sum  install.sh  README.md  AGENTS.md  CODEBASE.md
├── docs/  .github/  bin/
└── wol-agent-boot.service  wol-agent-shutdown.service
```

**不改变**：模块路径 `github.com/yusiwen/myUtilities`、CLI 命令名、网关路由、前端 embed 相对路径、ldflags、install.sh、用户可见行为。

## 改动规模

- 移动文件：~180 个 .go 文件 + 12 个 frontend 目录（含 dist）+ web/shared
- import 重写：42 个文件、103 处引用（`github.com/yusiwen/myUtilities/X` → `.../internal/X`）
- 命令包 23 个、core 子包 23 个、前端模块 12 个
- gateway 为"集线器"，import 12 个命令包，全部一并移入 internal/

> **`git mv` 关键前提**：`frontend/dist/`（12 个）与 `node_modules/` 被 gitignore，但 `git mv` 对目录执行 OS rename，这些被忽略文件会随目录一并移动。dist 是 `go build ./...`（embed 需要 dist 存在）能通过的前提。

## 执行步骤

### 前置 — 提交当前改动

- [x] 提交 4 个已修改文件（AGENTS.md、README.md、docs/scip-integration-plan.md、docs/tasks.md）
- [x] 一并提交本跟踪文档 `docs/cmd-internal-restructure-plan.md`（untracked）

### Commit 1 — 目录迁移 + import 重写

- [ ] `mkdir -p cmd/mu && git mv main.go myutilities.go version.go cmd/mu/`
- [ ] 23 个命令包 `git mv` 到 `internal/`
- [ ] `git mv core internal/core`
- [ ] `git mv shared web/shared`
- [ ] 全局 sed 重写 import（42 文件 / 103 处加 `internal/` 前缀）
- [ ] 验证：`go build ./cmd/mu` + `go vet ./...` + `go test ./...`

### Commit 2 — 构建系统

- [ ] Makefile **两处独立构建路径**：
  - `GOBUILD`（line 21）`go build ... .` → `./cmd/mu`（平台 target 全部复用此变量，无需逐个改）
  - `default:`（line 32）单独 `go build -o bin/mu .` → `./cmd/mu`（保持无 ldflags 注入版本的既有行为）
- [ ] Makefile：`FRONTEND_DIRS`（12 项）+ 12 个 `*_FRONTEND_DIR` 加 `internal/` 前缀
- [ ] Makefile：`THEME_PARTIAL`/`COMMON_PARTIAL` → `web/shared/frontend/`
- [ ] .gitignore：**24 行** frontend 条目（12 模块 × `node_modules/` + `dist/`）→ `internal/*/frontend/...`；其余条目（`tmp/`、`hosts.json`、`mu` 等）与目录无关，不动
- [ ] 验证：`make build` + `./bin/mu --help` + `./bin/mu --version` 冒烟

### Commit 3 — 文档

- [ ] AGENTS.md：项目布局图、core/→internal/core/、shared partials 路径
- [ ] README.md：**确认无需更新**（12 处 `core/`/`wol/` 等匹配均为 API 路由/URL，无源码布局引用）
- [ ] CODEBASE.md：目录树更新
- [ ] docs/codebase-restructure-plan.md：Phase 3 标记完成、修正影响评估
- [ ] docs/tasks.md：勾选布局重构项
- [ ] 验证：grep 确认无 `myUtilities/<pkg>` 旧路径残留于文档

### 收尾

- [ ] 删除根目录 `~/` 残留目录（untracked）、`myUtilities` 编译产物（git 已忽略）
- [ ] CI（`make all`）最终验证

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| import 漏改导致编译失败 | `go build ./...` 编译器逐个定位，修复量极小 |
| `//go:embed` 路径失效 | embed 相对包自身，frontend/ 随包移动；已核实 14 处 embed（含 mock/oauth 的 templates+static） |
| `git mv` 未带上被忽略的 dist/ | `git mv` 是 OS rename 整目录，dist/node_modules 一并移动；build 前 `ls internal/*/frontend/dist` 复核 |
| Makefile 路径遗漏 | 仅 GOBUILD + default: 两行构建路径 + frontend 变量，`grep -n "frontend" Makefile` 逐处核对 |
| 包名冲突（core/crypto vs crypto 等） | 本次不处理，alias 保留，行为不变 |
| 回滚 | 3 个 commit 独立，各自可回退 |

## 进度

- [ ] 前置：提交当前改动
- [ ] Commit 1：目录迁移 + import 重写
- [ ] Commit 2：构建系统
- [ ] Commit 3：文档
- [ ] 收尾：清理 + CI 验证
