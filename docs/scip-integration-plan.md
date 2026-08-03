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
    // 生成命令由这些字段数据驱动，便于支持不同 indexer 的 CLI：
    Prefix     []string // 固定前置参数，如 ["scip"]（rust-analyzer scip）
    OutputFlag string   // 指定输出路径的 flag，如 "-o"（scip-go）、"--output"（rust-analyzer）
    Trailing   []string // 输出路径之后追加的参数，如 ["."]（rust-analyzer scip）
}
```

初始注册表（**仅零摩擦的 GitHub release 分发**）：

| 语言 | Indexer | 仓库 | 分发 | 状态 |
|------|---------|------|------|------|
| Go | scip-go | `scip-code/scip-go` | GitHub release 二进制 | **启用（v1）** |
| Rust | rust-analyzer（`scip` 子命令） | `rust-lang/rust-analyzer` | GitHub release 二进制（裸 `.gz`） | **启用（v1.5）** |
| TypeScript/JS | scip-typescript | `scip-code/scip-typescript` | npm | 注册但需 Node，v2 |
| Java | scip-java | `scip-code/scip-java` | GitHub release（JVM launcher，资产无 OS/arch） | **已启用**：按名下载 + review 自动构建 |
| C/C++ | scip-clang | `scip-code/scip-clang` | GitHub release | 注册但需 compile_commands.json，默认禁用 |

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
Cargo.toml                 → rust
*.rs（大量）               → rust
package.json + tsconfig.json → typescript
pom.xml                    → java
settings.gradle / gradlew / build.gradle(.kts) → java
*.java（大量）             → java
compile_commands.json      → clang
```

返回 `[]string`（一个仓库可多语言），未匹配到任何语言返回空（此时 review 不启用语义工具，静默回退）。

## 3.1 Rust 支持方案

### 调研结论

- **没有独立的 scip-rust 二进制**：`scip-code/scip-rust` 仅是一个 shell 包装脚本
  （release `v0.0.6` **无 release 资产**），底层命令就是 `rust-analyzer scip`。
- **rust-analyzer 是可行的分发载体**：`rust-lang/rust-analyzer` 发布 GitHub release
  二进制（如 `rust-analyzer-x86_64-unknown-linux-gnu.gz`，裸 `.gz` 单文件），且
  `rust-analyzer scip` 支持 `--output PATH` 参数，可直接对接现有自动下载框架。
- 包装脚本确认所需工具：`cargo`、`rustc`、`rust-analyzer` 三者必须在 PATH；
  `rust-analyzer scip --output <path> <repoRoot>` 即完整索引命令。

### 注册表条目

```go
{
    Lang:       "rust",
    Detect:     []string{"Cargo.toml", "*.rs"},
    GitHubRepo: "rust-lang/rust-analyzer",
    Version:    "2026-07-27",            // rust-analyzer 用日期标签
    Install:    MethodGitHubRelease,
    Requires:   []string{"cargo", "rustc"}, // rust-analyzer scip 需加载 workspace
    BinaryName: "rust-analyzer",
    OutputFile: "index.scip",
    Prefix:     []string{"scip"},
    OutputFlag: "--output",
    Trailing:   []string{"."},
}
```

生成命令展开为：`rust-analyzer scip --output <outPath> [-q] .`（cwd = repoRoot）。

### 需要的基础设施改动

| 改动 | 文件 | 说明 |
|------|------|------|
| 生成命令数据驱动 | `core/scip/runner.go` `generate()` | 目前硬编码 `scip-go -o <path> -q`；改为按 `Indexer.Prefix/OutputFlag/Trailing` 拼装，scip-go 对应 `Prefix=[] OutputFlag="-o" Trailing=[]`，行为不变 |
| 支持裸 `.gz` 单文件二进制 | `core/scip/toolchain.go` `extractBinary()` | rust-analyzer 资产是裸 gzip 二进制而非 tar.gz；`.gz` 先试 tar 解析，失败则按单个二进制直接解压到目标名 |

### 运行时前提与已知限制

- 索引时 `cargo` + `rustc` 必须在 PATH（`rust-analyzer scip` 要解析 cargo workspace）
- rust-analyzer 的 `scip` 子命令较新，宏/泛型的符号精度可能不如 scip-go 成熟
- Windows 资产被现有 `core/installer` 过滤（所有 indexer 的既有约束）

### 验证计划

1. toolchain 单测：构造裸 `.gz` 二进制走解压路径
2. registry 单测：`LookupLang("rust")` 存在且非 Disable
3. 集成测试（可选）：临时 cargo 工程跑通 `EnsureIndex` + `FindDefinition`

## 3.2 Java 支持方案

### 调研结论

- **scip-java 是 JVM 应用**（Scala 编写，发布到 Maven Central `org.scip-code:scip-java`），
  GitHub release 资产 `scip-java-v0.13.1` 是 Coursier 构建的**跨平台 JVM launcher**
  （名字不含 OS/arch，仍依赖 JDK 17+ 运行）。
- **与 Go 的本质差异**：`scip-java index` 会**真正执行构建**——
  Gradle `clean compileTestJava...` / Maven `--batch-mode clean verify -DskipTests`，
  有副作用（清编译缓存、下载全部依赖、向 cwd 写 `index.scip`），耗时分钟级。
- **构建工具支持**：自动配置仅 Maven + Gradle（Java）；Kotlin 仅 Gradle；Bazel 需
  特殊处理（`--bazel-scip-java-binary` 参数 / aspect）；Ant/Buck 不支持。
- **JVM 选项**：JDK 17/21/25 需要 `--add-exports` 访问 javac 内部 API，launcher 已内置，
  使用方无需手动配置。

### 与 scip-go 的对比

| 维度 | Go (scip-go) | Java (scip-java) |
|------|-------------|-----------------|
| 本体 | 原生二进制 | JVM launcher（跨平台，需 JDK 17+） |
| 运行方式 | 只读跑 `go list`，秒级 | `index` 子命令执行真实构建，分钟级 |
| 副作用 | 无 | 清编译缓存、下载依赖、cwd 写 `index.scip` |
| 资产命名 | 带 OS/arch | 无 OS/arch（现有 `core/installer` 匹配会失效） |
| 运行时依赖 | `go` | JDK 17+ + Maven/Gradle（或项目内 gradlew/mvnw） |

### 注册表条目（已实现）

```go
{
    Lang:       "java",
    Detect:     []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "gradlew", "*.java"},
    GitHubRepo: "scip-code/scip-java",
    Version:    "v0.13.1",
    Install:    MethodGitHubRelease,
    Requires:   []string{"java"},          // + 构建工具（mvn/gradle 或项目 wrapper，scip-java 自动检测）
    BinaryName: "scip-java",
    OutputFile: "index.scip",
    Prefix:     []string{"index"},         // scip-java index
    OutputFlag: "--output",                 // 已确认支持
    AssetName:  "scip-java-v0.13.1",        // 无 OS/arch，按名下载
    FailHard:   true,                       // 构建失败时 review 直接退出
}
```

生成命令展开为：`scip-java index --output <outPath>`（cwd = repoRoot），indexer 输出捕获到临时文件。

### 需要的基础设施改动（均已实现）

| 改动 | 文件 | 说明 |
|------|------|------|
| 资产按名下载 | `core/scip/toolchain.go` | 新增 `Indexer.AssetName` 字段 + `Install()` 按名下载分支（跨平台 launcher，下载即用无需解压）；`core/installer` 新增 `AssetByURL` |
| 参数数据驱动 | `core/scip/runner.go` | `buildArgs()` 按 `Prefix/OutputFlag/QuietFlag/Trailing` 拼装；go 行为不变（`-o <path> -q`） |
| 输出统一捕获 | `core/scip/runner.go` | `runIndexer()`：默认 indexer 输出进临时文件；`--verbose` 直接流式；失败保留文件 + 提取错误行 |
| 构建信息行 + spinner | `core/scip/runner.go` | 构建/重建时打印一行 + spinner（非 TTY 仅打印行）；重建原因行（stale / forced） |
| 失败即退出 | `core/scip/runner.go` + `git/review.go` | `IndexError{Lang, Err, Hard}`；java `FailHard=true`，review 收到 Hard 错误直接 `return err`（fail fast），go 维持降级 |

### 使用方需要具备的条件

1. **JDK 17+**（scip-java 本体 + 被索引项目编译都需要）
2. **Maven 或 Gradle 工程**：根目录有 `pom.xml` 或 `settings.gradle`/`gradlew`/`build.gradle(.kts)`
3. **构建必须能跑通**（索引时实际执行编译类命令，编译失败则索引失败）
4. **依赖可解析**：首次索引需联网下载项目全部外部依赖（Maven Central）
5. **时间预期**：首次索引分钟级，不是 Go 的秒级体验
6. `--add-exports` JVM 选项已内置在 launcher，**用户无需手动配置**
7. Windows 仍受 `core/installer` 资产过滤约束（既有限制）

### review 内 java 行为（已确认）

| 场景 | 行为 |
|------|------|
| 无索引 | 直接构建（无确认），打印「Indexing java ...」+ spinner |
| 索引过期 | 不提示，打印「Rebuilding java index: index is stale」直接重建 + spinner |
| `--refresh-scip` | 打印「Rebuilding java index (forced by --refresh-scip)」强制重建 |
| 构建失败 | **报错退出**，信息含失败原因 + `Full indexer log kept at: <path>` |

### 验证计划

1. toolchain 单测：按 `AssetName` 下载 JVM launcher 并校验可执行 ✅
2. registry 单测：`LookupLang("java")` 存在、非 Disable、`FailHard=true` ✅
3. `buildArgs` / `IndexError` 单测 ✅
4. 集成验证：临时 Maven 工程跑通 `EnsureIndex` + `FindDefinition` ✅（已实测，kind=StaticMethod、签名正确）

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
| 7 | Rust 支持：registry 加 rust + `generate()` 数据驱动 + 裸 `.gz` 解压 | ~80 行 |
| 8 | Java 支持：`AssetName` 按名下载 + registry 启用 + `generate()` 适配 + 自动构建/fail-fast | ~120 行 |

## 11. 风险与权衡

| 风险 | 缓解 |
|------|------|
| indexer 分发方式不一（npm/pip/gem） | 注册表 `InstallMethod` 抽象；v1 仅承诺 GitHub release（Go/Rust/Java/Clang）；TS 走 npx 归入 v2 |
| scip-go 运行时需 `go` 在 PATH | `Requires` 字段 + preflight 检查，缺失时降级提示 |
| rust-analyzer scip 运行时需 `cargo`+`rustc` | `Requires: ["cargo","rustc"]` + preflight 检查；rust-analyzer 自身自动下载 |
| scip-java 资产名无 OS/arch | `Indexer.AssetName` 按名下载分支，绕过 `core/installer` 的 OS/arch 过滤 |
| scip-java 运行时需 JDK 17+ + 构建工具 | `Requires: ["java"]` + 构建工具/项目 wrapper 检测；缺失则跳过 Java 并提示 |
| scip-java 每次索引执行全量构建（分钟级、有副作用） | review **自动构建**（无索引/过期即建，fail-fast），打印原因行 + spinner；`--refresh-scip` 强制重建；构建输出进临时文件 |
| 首次 review 下载+索引延迟 | commit 缓存复用；verbose 打印进度；`--refresh-scip` 手动控制 |
| 大仓 dirty 反复重索引 | 按 commit 缓存 + 仅 dirty 时重生成 working 索引；dirty 时按源文件 mtime 新鲜度决定是否重建 |
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
| 7 | Rust 支持（registry + 数据驱动 generate + 裸 `.gz` 解压） | ⬜ 待实施 |
| 8 | Java 支持（AssetName 下载 + buildArgs + 输出临时文件 + FailHard + spinner） | ✅ |
