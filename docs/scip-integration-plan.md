# SCIP 语义索引集成计划

## 背景

`mu git review` 的 agent（`core/git/agent.go`）现有 4 个工具都是**文本/正则级别**的：

| 工具 | 现状 | 缺陷 |
|------|------|------|
| `read_function` | 目标行 ±30 行窗口（agent.go:442） | 不是真实函数边界，大函数/紧凑代码会丢失上下文 |
| `search_code` | `git grep` 正则匹配（agent.go:424） | 无法区分定义/引用；找不到跨文件 caller；重名符号混淆 |
| `read_file` | 纯文本读取 | LLM 看到 `foo.Bar()` 不知道 Bar 是什么、定义在哪 |
| `read_diff` | git diff | 无符号级信息 |

目标是引入 **Sourcegraph SCIP**（LSIF 的升级，语言无关的符号索引协议）生成语义数据，让 agent 通过新的语义工具精确掌握项目代码，重点是**改动影响面分析**（改签名/删函数时定位全仓调用方）。

## 总体架构

```
mu scip
  ├── install <lang>   按需下载 indexer（treesitter-nvim 式自动安装）
  ├── list             列出已安装 / 可用 indexer
  ├── index            手动生成当前仓库索引
  └── purge            清空索引 / 工具缓存

git review（首次自动触发）
  └── detect 语言 → 下载 indexer → 生成索引 → 缓存
                        ↓
            agent 语义工具（find_references / find_definition /
            symbol_info / read_function 升级）
```

### 核心思路（与 treesitter-nvim 一致）

- 根据项目代码**自动识别语言**（`go.mod`、`package.json`、`pom.xml`、扩展名…）
- 缺 indexer 时**自动从 GitHub release 下载**对应 CLI 到工具缓存目录
- 索引按 **commit 缓存**，命中直接复用；dirty 工作区生成 `working` 索引
- 复用现有 `core/installer.Client.QueryAssets()`（已实现按 OS/arch 解析 GitHub release 二进制资产）

## 目录结构

```
core/scip/
  ├── registry.go     语言 → indexer 注册表（检测信号 + InstallMethod）
  ├── toolchain.go    下载 / 缓存 / 定位 indexer 二进制
  ├── detect.go       从 repo 根检测项目语言
  ├── runner.go       调用 indexer 生成 .scip，commit 缓存 + 锁
  ├── index.go        加载 SCIP protobuf + 语义查询 API
  └── index_test.go   查询 API 单元测试

scip/
  ├── options.go      install / list / index / purge 参数
  └── command.go      CLI 入口
```

## 1. 注册表 — `core/scip/registry.go`

```go
type InstallMethod string

const (
    MethodGitHubRelease InstallMethod = "github_release" // 复用 core/installer，自动下载
    MethodNpm           InstallMethod = "npm"            // 未来：npx/npm -g（需 Node）
    MethodPip           InstallMethod = "pip"            // 未来（需 Python）
)

type Indexer struct {
    Lang         string        // "go"
    Detect       []string      // ["go.mod", "*.go"]
    GitHubRepo   string        // "sourcegraph/scip-go"（GitHubRelease 用）
    Version      string        // 版本 pin，如 "v0.4.0"
    Install      InstallMethod
    Requires     []string      // 运行时依赖，如 ["go"]（scip-go 内部跑 go list）
    OutputFormat string        // "scip"（单文件 index.scip）
    Disable      bool          // 需要构建系统（scip-java/clang），默认不启用
}
```

初始注册表（**仅零摩擦的 GitHub release 分发**）：

| 语言 | Indexer | 仓库 | 分发 | 状态 |
|------|---------|------|------|------|
| Go | scip-go | `sourcegraph/scip-go` | GitHub release 二进制 | **启用（v1 唯一支持）** |
| TypeScript/JS | scip-typescript | `sourcegraph/scip-typescript` | npm | 注册但需 Node，v2 |
| Java | scip-java | `sourcegraph/scip-java` | GitHub release | 注册但需 JVM+构建，默认禁用 |
| C/C++ | scip-clang | `sourcegraph/scip-clang` | GitHub release | 注册但需 compile_commands.json，默认禁用 |

## 2. 工具链 — `core/scip/toolchain.go`（treesitter-nvim 式自动安装）

```
工具缓存: ~/.cache/mu/scip/tools/<name>/<version>/<binary>
```

流程：

1. `Lookup(name)` — 检查缓存中是否存在可执行二进制，命中直接返回路径
2. 未命中 → 复用 `core/installer.Client.QueryAssets(Query{User,Program,Release})` 解析 release 资产（scip-go 发布 `scip-go_linux_amd64` 等命名，符合现有 `GetOS`/`GetArch` 解析）
3. 下载 → 解压 → 校验 SHA256 → `chmod +x` → 写入版本化目录
4. 打印安装信息（名称/版本/路径），verbose 模式显示详情

`scip install <lang>`：手动安装指定语言 indexer。
`scip list`：列出注册表 + 已安装状态 + 缺失项。

## 3. 语言检测 — `core/scip/detect.go`

从 repo 根按注册表 `Detect` 信号匹配：

```
go.mod                     → go
*.go（大量）               → go
package.json + tsconfig.json → typescript
pom.xml                    → java
compile_commands.json      → clang
```

返回 `[]string`（一个仓库可多语言），未匹配到任何语言返回空（此时 review 不启用语义工具，静默回退）。

## 4. 索引生成与缓存 — `core/scip/runner.go`

### 缓存布局

```
~/.cache/mu/scip/index/<project>/<lang>/<commit>.scip     # commit 命中复用
~/.cache/mu/scip/index/<project>/<lang>/working.scip      # dirty 工作区
~/.cache/mu/scip/index/<project>/<lang>/.lock             # 生成锁（防并发）
```

### 策略

| 场景 | 行为 |
|------|------|
| 工作区干净，commit 有缓存 | 直接复用，跳过生成 |
| 工作区干净，commit 无缓存 | 生成并缓存 |
| 工作区 dirty | 生成 `working.scip`（覆盖旧文件），生成期间持锁 |
| 索引生成失败 | 记录日志，review 回退到现有文本工具，不阻断 |

### scip-go 调用

```
scip-go --from-module 或模块目录内直接执行 → 产出 index.scip
```

生成前 `CheckPreflight`：二进制存在 + `Requires` 依赖（`go`）在 PATH。

## 5. 查询 API — `core/scip/index.go`

依赖 `github.com/sourcegraph/scip/bindings/go`（SCIP protobuf 官方 Go 绑定）。

```go
type Index struct {
    docs    map[string]*scip.Document        // path → document
    symbols map[string][]*scip.Occurrence    // symbol → 全部 occurrences
}

func Load(path string) (*Index, error)

// FindDefinition 返回给定行上符号的定义位置（跨文件）
func (ix *Index) FindDefinition(path string, line int) ([]Location, error)

// FindReferences 返回某符号的全部引用位置（含定义，跨文件）
func (ix *Index) FindReferences(path string, line int) ([]Location, error)

// SymbolsInRange 返回 diff 改动行范围内的所有符号
func (ix *Index) SymbolsInRange(path string, start, end int) ([]SymbolInfo, error)

// SymbolInfo 返回符号的签名/类型/文档（来自 symbol_information）
func (ix *Index) SymbolInfo(symbol string) (*SymbolInfo, error)
```

内部细节：

- SCIP occurrence 用 UTF-8 byte offset（`StartCharacter`/`EndCharacter`），查询时读文件做 **offset ↔ 行号映射**（缓存每文件的行起始 offset 表）
- 多语言索引：`IndexSet` 持有 `map[lang]*Index`，按文件扩展名路由
- `FindReferences` = 收集 symbol 下 role != Definition 的 occurrence（跨 document）

## 6. Agent 工具接入 — `core/git/agent.go`

### 新增工具

| 工具 | 参数 | 说明 |
|------|------|------|
| `find_references` | `path`, `line` | 某位置的符号在**全仓**的引用/调用点（跨文件），review 影响面分析核心 |
| `find_definition` | `path`, `line` | 跳转到该行引用的符号的定义处，返回定义文件+上下文 |
| `symbol_info` | `path`, `line` | 符号签名、类型、文档（hover） |

### 升级工具

| 工具 | 变化 |
|------|------|
| `read_function` | 优先用 SCIP occurrence 定位真实函数起止行；无索引时回退 ±30 行窗口 |

### 优雅降级

- 索引不存在 / 未匹配到语言 / 生成失败 → 工具返回提示并回退现有行为（`search_code` 保留，`read_function` 回退窗口）
- 符号在索引中未找到 → 返回「symbol not found in index」
- `NewReviewAgent` 增加可选的 `*scip.IndexSet` 字段；构造时调用 `scip.Ensure(repoRoot, cacheDir)`（检测→下载→生成，幂等）

### token 成本

SCIP 查询结果仅返回 `文件:行` + 符号名，紧凑低 token；配合现有 `truncateToolResult`（30000 字符上限）安全。

## 7. CLI — `scip/`

```bash
# 手动生成当前仓库索引
mu scip index

# 按语言安装 indexer（treesitter-nvim 式）
mu scip install go
mu scip install typescript   # v2，需 Node

# 查看可用 / 已安装
mu scip list

# 清空工具 + 索引缓存
mu scip purge

# review 时显式控制
mu git review --no-scip           # 禁用语义工具
mu git review --refresh-scip      # 强制重新生成（大仓 dirty 时用）
```

注册到 `myutilities.go`：

```go
Scip ScipCmd `cmd:"" name:"scip" help:"SCIP semantic code intelligence."`
```

## 8. 配置 — `git-config.json` review 模块

```json
{
  "review": {
    "provider": "default",
    "lang": "en",
    "scip": {
      "enabled": true,
      "auto_install": true,
      "cache_dir": ""
    }
  }
}
```

- 遵循现有约定：`LoadConfig`/`RegisterHandlers` 接受 config 路径参数，空则回退 `~/.config/mu/git-config.json`
- `cache_dir` 为空 → `~/.cache/mu/scip`
- `enabled=false` 或 `auto_install=false` 时 review 静默回退现有文本工具

## 9. 新增依赖

```
github.com/sourcegraph/scip/bindings/go   SCIP protobuf 官方 Go 绑定
（其余复用已有 core/installer，无新二进制/CGo，保持跨平台编译）
```

## 10. 实现顺序

| 阶段 | 内容 | 预计改动量 |
|------|------|-----------|
| 1 | `core/scip/registry.go` + `toolchain.go`（注册表 + 自动下载，复用 core/installer） | ~200 行 |
| 2 | `core/scip/detect.go` + `runner.go`（语言检测 + commit 缓存 + 锁） | ~200 行 |
| 3 | `core/scip/index.go` + `index_test.go`（SCIP 加载 + 4 个查询 API） | ~350 行 |
| 4 | `core/git/agent.go` 接入（3 新工具 + read_function 升级 + 降级） | ~200 行 |
| 5 | `scip/` CLI + `git-config.json` scip 配置 + review 集成（`--no-scip`/`--refresh-scip`） | ~250 行 |
| 6 | README 文档 + `docs/tasks.md` 更新 + 全项目编译/测试/lint | — |

## 11. 风险与权衡

| 风险 | 缓解 |
|------|------|
| indexer 分发方式不一（npm/pip/gem） | 注册表 `InstallMethod` 抽象；v1 仅承诺 GitHub release（Go/Java/Clang）；TS 走 npx 归入 v2 |
| scip-go 运行时需 `go` 在 PATH | `Requires` 字段 + preflight 检查，缺失时降级提示 |
| 首次 review 下载+索引延迟 | commit 缓存复用；verbose 打印进度；`--refresh-scip` 手动控制 |
| 大仓 dirty 反复重索引 | 按 commit 缓存 + 仅 dirty 时重生成 working 索引 |
| 行号↔offset 转换错误 | 每文件缓存行起始 offset 表，单元测试覆盖多字节字符（中文注释） |
| 与 `docs/tree-sitter-wasm-plan.md` 重叠 | SCIP 方案 supersede 该文件中的 Option A/C（find_definition 目标），完成后可在该文档标注已由 SCIP 实现 |

## 12. 实现进度

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | registry + toolchain | ✅ |
| 2 | detect + runner | ✅ |
| 3 | index 查询 API + 测试 | ✅ |
| 4 | agent 工具接入 | ✅ |
| 5 | CLI + 配置 | ✅ |
| 6 | 文档 + 验证 | ✅ |
