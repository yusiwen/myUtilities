# SCIP Semantic Index Integration Plan

## Background

`mu git review`'s agent (`core/git/agent.go`) currently has 4 tools that are all **text/regex-level**:

| Tool | Current state | Limitation |
|------|------|------|
| `read_function` | Target line ±30 line window (agent.go:442) | Not true function boundaries, large functions/compact code loses context |
| `search_code` | `git grep` regex matching (agent.go:424) | Cannot distinguish definition/references; cannot find cross-file callers;同名 symbol confusion |
| `read_file` | Plain text read | LLM sees `foo.Bar()` but doesn't know what Bar is or where it's defined |
| `read_diff` | git diff | No symbol-level information |

Goal is to introduce **Sourcegraph SCIP** (an upgrade to LSIF, a language-agnostic symbol indexing protocol) to generate semantic data, allowing the agent to precisely understand project code through new semantic tools, with a focus on **change impact analysis** (locating all callers in the repo when changing signatures/deleting functions).

## Overall Architecture

```
mu scip
  ├── install <lang>   On-demand indexer download (treesitter-nvim-style auto-install)
  ├── list             List installed / available indexers
  ├── index            Manually generate index for current repo
  └── purge            Clear indexes / tool cache

git review (auto-triggered on first use)
  └── detect language → download indexer → generate index → cache
                         ↓
            agent semantic tools (find_references / find_definition /
            symbol_info / read_function upgraded)
```

### Core Approach (consistent with treesitter-nvim)

- **Auto-detect language** based on project code (`go.mod`, `package.json`, `pom.xml`, extensions...)
- When indexer is missing, **auto-download** the corresponding CLI from GitHub release to the tool cache directory
- Indexes are **cached per commit**, hit directly to reuse; dirty workspace generates a `working` index
- Reuse existing `core/installer.Client.QueryAssets()` (already implements parsing GitHub release binary assets by OS/arch)

## Directory Structure

```
core/scip/
  ├── registry.go     Language → indexer registry (detection signals + InstallMethod)
  ├── toolchain.go    Download / cache / locate indexer binary
  ├── detect.go       Detect project language from repo root
  ├── runner.go       Call indexer to generate .scip, commit cache + lock
  ├── index.go        Load SCIP protobuf + semantic query API
  └── index_test.go   Query API unit tests

scip/
  ├── options.go      install / list / index / purge options
  └── command.go      CLI entry point
```

## 1. Registry — `core/scip/registry.go`

```go
type InstallMethod string

const (
    MethodGitHubRelease InstallMethod = "github_release" // reuse core/installer, auto-download
    MethodNpm           InstallMethod = "npm"            // future: npx/npm -g (requires Node)
    MethodPip           InstallMethod = "pip"            // future (requires Python)
)

type Indexer struct {
    Lang         string        // "go"
    Detect       []string      // ["go.mod", "*.go"]
    GitHubRepo   string        // "sourcegraph/scip-go" (for GitHubRelease)
    Version      string        // version pin, e.g. "v0.4.0"
    Install      InstallMethod
    Requires     []string      // runtime dependencies, e.g. ["go"] (scip-go internally runs go list)
    OutputFormat string        // "scip" (single file index.scip)
    Disable      bool          // requires build system (scip-java/clang), disabled by default
    // Generation commands are data-driven by these fields, supporting different indexer CLIs:
    Prefix     []string // fixed prefix parameters, e.g. ["scip"] (rust-analyzer scip)
    OutputFlag string   // flag to specify output path, e.g. "-o" (scip-go), "--output" (rust-analyzer)
    Trailing   []string // parameters appended after output path, e.g. ["."] (rust-analyzer scip)
}
```

Initial registry (**zero-friction GitHub release distribution only**):

| Language | Indexer | Repo | Distribution | Status |
|------|---------|------|------|------|
| Go | scip-go | `scip-code/scip-go` | GitHub release binary | **Enabled (v1)** |
| Rust | rust-analyzer (`scip` subcommand) | `rust-lang/rust-analyzer` | GitHub release binary (bare `.gz`) | **Enabled (v1.5)** |
| TypeScript/JS | scip-typescript | `scip-code/scip-typescript` | npm | Registered but requires Node, v2 |
| Java | scip-java | `scip-code/scip-java` | GitHub release (JVM launcher, asset has no OS/arch) | **Enabled**: download by name + review auto-build |
| C/C++ | scip-clang | `scip-code/scip-clang` | GitHub release | Registered but requires compile_commands.json, disabled by default |

## 2. Toolchain — `core/scip/toolchain.go` (treesitter-nvim-style auto-install)

```
Tool cache: ~/.cache/mu/scip/tools/<name>/<version>/<binary>
```

Flow:

1. `Lookup(name)` — check if executable binary exists in cache, hit returns path directly
2. Miss → reuse `core/installer.Client.QueryAssets(Query{User,Program,Release})` to parse release assets (scip-go publishes `scip-go_linux_amd64` etc., compatible with existing `GetOS`/`GetArch` parsing)
3. Download → extract → verify SHA256 → `chmod +x` → write to versioned directory
4. Print install info (name/version/path), verbose mode shows details

`scip install <lang>`: manually install indexer for specified language.
`scip list`: list registry + installed status + missing items.

## 3. Language Detection — `core/scip/detect.go`

Match from repo root by registry `Detect` signals:

```
go.mod                     → go
*.go (many)                → go
Cargo.toml                 → rust
*.rs (many)                → rust
package.json + tsconfig.json → typescript
pom.xml                    → java
settings.gradle / gradlew / build.gradle(.kts) → java
*.java (many)              → java
compile_commands.json      → clang
```

Returns `[]string` (a repo can have multiple languages), returns empty if no language matches (at this point review does not enable semantic tools, silently falls back).

## 3.1 Rust Support Plan

### Research Findings

- **No independent scip-rust binary**: `scip-code/scip-rust` is just a shell wrapper script
  (release `v0.0.6` **has no release assets**), the underlying command is `rust-analyzer scip`.
- **rust-analyzer is a viable distribution carrier**: `rust-lang/rust-analyzer` publishes GitHub release
  binaries (e.g., `rust-analyzer-x86_64-unknown-linux-gnu.gz`, bare `.gz` single file), and
  `rust-analyzer scip` supports `--output PATH` parameter, can directly connect to existing auto-download framework.
- Wrapper script confirms required tools: `cargo`, `rustc`, `rust-analyzer` all must be in PATH;
  `rust-analyzer scip --output <path> <repoRoot>` is the complete index command.

### Registry Entry

```go
{
    Lang:       "rust",
    Detect:     []string{"Cargo.toml", "*.rs"},
    GitHubRepo: "rust-lang/rust-analyzer",
    Version:    "2026-07-27",            // rust-analyzer uses date labels
    Install:    MethodGitHubRelease,
    Requires:   []string{"cargo", "rustc"}, // rust-analyzer scip needs to load workspace
    BinaryName: "rust-analyzer",
    OutputFile: "index.scip",
    Prefix:     []string{"scip"},
    OutputFlag: "--output",
    Trailing:   []string{"."},
}
```

Generation command expands to: `rust-analyzer scip --output <outPath> [-q] .` (cwd = repoRoot).

### Required Infrastructure Changes

| Change | File | Description |
|------|------|------|
| Data-driven generation command | `core/scip/runner.go` `generate()` | Currently hardcoded `scip-go -o <path> -q`; change to assemble by `Indexer.Prefix/OutputFlag/Trailing`, scip-go corresponds to `Prefix=[] OutputFlag="-o" Trailing=[]`, behavior unchanged |
| Support bare `.gz` single-file binary | `core/scip/toolchain.go` `extractBinary()` | rust-analyzer assets are bare gzip binaries not tar.gz; `.gz` first tries tar parse, on failure extracts as single binary directly to target name |

### Runtime Prerequisites and Known Limitations

- `cargo` + `rustc` must be in PATH when indexing (`rust-analyzer scip` needs to parse cargo workspace)
- rust-analyzer's `scip` subcommand is relatively new, symbol precision for macros/generics may not be as mature as scip-go
- Windows assets are filtered by existing `core/installer` (existing constraints for all indexers)

### Verification Plan

1. toolchain unit test: construct bare `.gz` binary to test extraction path
2. registry unit test: `LookupLang("rust")` exists and is not Disable
3. Integration test (optional): temporary cargo project to run `EnsureIndex` + `FindDefinition`

## 3.2 Java Support Plan

### Research Findings

- **scip-java is a JVM application** (written in Scala, published to Maven Central `org.scip-code:scip-java`),
  GitHub release asset `scip-java-v0.13.1` is a Coursier-built **cross-platform JVM launcher**
  (name has no OS/arch, still requires JDK 17+ to run).
- **Essential difference from Go**: `scip-java index` **actually executes a build**——
  Gradle `clean compileTestJava...` / Maven `--batch-mode clean verify -DskipTests`,
  has side effects (clears compilation cache, downloads all dependencies, writes `index.scip` to cwd), takes minutes.
- **Build tool support**: Auto-configuration only for Maven + Gradle (Java); Kotlin only Gradle; Bazel needs
  special handling (`--bazel-scip-java-binary` parameter / aspect); Ant/Buck not supported.
- **JVM options**: JDK 17/21/25 need `--add-exports` to access javac internal API, the launcher has it built-in,
  users do not need to configure manually.

### Comparison with scip-go

| Dimension | Go (scip-go) | Java (scip-java) |
|------|-------------|-----------------|
| Nature | Native binary | JVM launcher (cross-platform, requires JDK 17+) |
| Execution | Read-only `go list`, seconds | `index` subcommand executes real build, minutes |
| Side effects | None | Clears compilation cache, downloads deps, writes `index.scip` to cwd |
| Asset naming | With OS/arch | No OS/arch (existing `core/installer` matching fails) |
| Runtime dependencies | `go` | JDK 17+ + Maven/Gradle (or project's gradlew/mvnw) |

### Registry Entry (implemented)

```go
{
    Lang:       "java",
    Detect:     []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "gradlew", "*.java"},
    GitHubRepo: "scip-code/scip-java",
    Version:    "v0.13.1",
    Install:    MethodGitHubRelease,
    Requires:   []string{"java"},          // + build tools (mvn/gradle or project wrapper, scip-java auto-detects)
    BinaryName: "scip-java",
    OutputFile: "index.scip",
    Prefix:     []string{"index"},         // scip-java index
    OutputFlag: "--output",                 // confirmed supported
    AssetName:  "scip-java-v0.13.1",        // no OS/arch, download by name
    FailHard:   true,                       // review exits directly on build failure
}
```

Generation command expands to: `scip-java index --output <outPath>` (cwd = repoRoot), indexer output captured to temp file.

### Required Infrastructure Changes (all implemented)

| Change | File | Description |
|------|------|------|
| Asset download by name | `core/scip/toolchain.go` | New `Indexer.AssetName` field + `Install()` download-by-name branch (cross-platform launcher, download and use directly without extraction); `core/installer` new `AssetByURL` (returns URL + companion `.sha256` checksum) + `fetchChecksum` |
| Data-driven parameters | `core/scip/runner.go` | `buildArgs()` assembles by `Prefix/OutputFlag/QuietFlag/Trailing`; Go behavior unchanged (`-o <path> -q`) |
| Unified output capture | `core/scip/runner.go` | `runIndexer()`: default indexer output goes to temp file; `--verbose` streams directly; failure keeps file + extracts error lines |
| Build info line + spinner | `core/scip/runner.go` | Print line + spinner on build/rebuild (non-TTY prints line only); rebuild reason line (stale / forced) |
| Failure exits | `core/scip/runner.go` + `git/review.go` | `IndexError{Lang, Err, Hard}`; Java `FailHard=true`, review receives Hard error and directly `return err` (fail fast), Go maintains fallback |

### Prerequisites for Users

1. **JDK 17+** (scip-java itself + indexed project compilation both need it)
2. **Maven or Gradle project**: root directory has `pom.xml` or `settings.gradle`/`gradlew`/`build.gradle(.kts)`
3. **Build must succeed** (indexing actually executes compilation commands, compilation failure means index failure)
4. **Dependencies resolvable**: First index needs network access to download all external dependencies (Maven Central)
5. **Time expectation**: First index is minutes, not Go's seconds experience
6. `--add-exports` JVM options are built into the launcher, **users do not need to configure manually**
7. Windows still subject to `core/installer` asset filtering constraints (existing limitations)

### Java behavior in review (confirmed)

| Scenario | Behavior |
|------|------|
| No index | Build directly (no confirmation), prints "Indexing java ..." + spinner |
| Index stale | No prompt, prints "Rebuilding java index: index is stale" and rebuilds directly + spinner |
| `--refresh-scip` | Prints "Rebuilding java index (forced by --refresh-scip)" forces rebuild |
| Build failure | **Exits with error**, info includes failure reason + `Full indexer log kept at: <path>` |

### Verification Plan

1. toolchain unit test: download JVM launcher by `AssetName` and verify executable ✅
2. registry unit test: `LookupLang("java")` exists, not Disable, `FailHard=true` ✅
3. `buildArgs` / `IndexError` / SHA256 verification (good/bad checksum) unit tests ✅
4. Integration verification: temporary Maven project to run `EnsureIndex` + `FindDefinition` ✅ (tested, kind=StaticMethod, signature correct)

## 4. Index Generation and Cache — `core/scip/runner.go`

### Cache Layout

```
~/.cache/mu/scip/index/<project>/<lang>/<commit>.scip     # commit hit reuse
~/.cache/mu/scip/index/<project>/<lang>/working.scip      # dirty workspace
~/.cache/mu/scip/index/<project>/<lang>/.lock             # generation lock (prevent concurrency)
```

### Strategy

| Scenario | Behavior |
|------|------|
| Workspace clean, commit has cache | Reuse directly, skip generation |
| Workspace clean, commit has no cache | Generate and cache |
| Workspace dirty | Generate `working.scip` (overwrite old file), hold lock during generation |
| Index generation fails | Log, review falls back to existing text tools, does not block |

### scip-go invocation

```
scip-go --from-module or execute directly in module directory → produce index.scip
```

Pre-flight `CheckPreflight`: binary exists + `Requires` dependencies (`go`) in PATH.

## 5. Query API — `core/scip/index.go`

Depends on `github.com/sourcegraph/scip/bindings/go` (SCIP protobuf official Go bindings).

```go
type Index struct {
    docs    map[string]*scip.Document        // path → document
    symbols map[string][]*scip.Occurrence    // symbol → all occurrences
}

func Load(path string) (*Index, error)

// FindDefinition returns the definition location of the symbol at a given line (cross-file)
func (ix *Index) FindDefinition(path string, line int) ([]Location, error)

// FindReferences returns all reference locations of a symbol (including definition, cross-file)
func (ix *Index) FindReferences(path string, line int) ([]Location, error)

// SymbolsInRange returns all symbols within the diff change line range
func (ix *Index) SymbolsInRange(path string, start, end int) ([]SymbolInfo, error)

// SymbolInfo returns the symbol's signature/type/documentation (from symbol_information)
func (ix *Index) SymbolInfo(symbol string) (*SymbolInfo, error)
```

Internal details:

- SCIP occurrence uses UTF-8 byte offset (`StartCharacter`/`EndCharacter`), query reads file to make **offset ↔ line number mapping** (cache each file's line start offset table)
- Multi-language index: `IndexSet` holds `map[lang]*Index`, routes by file extension
- `FindReferences` = collect occurrences under symbol where role != Definition (cross-document)

## 6. Agent Tool Integration — `core/git/agent.go`

### New Tools

| Tool | Parameters | Description |
|------|------|------|
| `find_references` | `path`, `line` | References/call sites of the symbol at a location **across the entire repo** (cross-file), core for review impact analysis |
| `find_definition` | `path`, `line` | Navigate to the definition of the symbol referenced at that line, returns definition file + context |
| `symbol_info` | `path`, `line` | Symbol signature, type, documentation (hover) |

### Upgraded Tools

| Tool | Changes |
|------|------|
| `read_function` | Priority use SCIP occurrence to locate true function start/end lines; fallback to ±30 line window when no index |

### Graceful Fallback

- Index not found / no language matched / generation failed → tools return hint and fall back to existing behavior (`search_code` retained, `read_function` falls back to window)
- Symbol not found in index → returns "symbol not found in index"
- `NewReviewAgent` adds optional `*scip.IndexSet` field; calls `scip.Ensure(repoRoot, cacheDir)` on construction (detect → download → generate, idempotent)

### Token cost

SCIP query results only return `file:line` + symbol name, compact and low token; safe with existing `truncateToolResult` (30000 character limit).

## 7. CLI — `scip/`

```bash
# Manually generate index for current repo
mu scip index

# Install indexer by language (treesitter-nvim style)
mu scip install go
mu scip install typescript   # v2, requires Node

# View available / installed
mu scip list

# Clear tool + index cache
mu scip purge

# Explicit control during review
mu git review --no-scip           # disable semantic tools
mu git review --refresh-scip      # force regenerate (use when large repo is dirty)
```

Register in `myutilities.go`:

```go
Scip ScipCmd `cmd:"" name:"scip" help:"SCIP semantic code intelligence."`
```

## 8. Configuration — `git-config.json` review module

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

- Follows existing conventions: `LoadConfig`/`RegisterHandlers` accepts config path parameter, falls back to `~/.config/mu/git-config.json` when empty
- `cache_dir` empty → `~/.cache/mu/scip`
- `enabled=false` or `auto_install=false` → review silently falls back to existing text tools

## 9. New Dependencies

```
github.com/sourcegraph/scip/bindings/go   SCIP protobuf official Go bindings
(rest reuse existing core/installer, no new binaries/CGo, maintain cross-platform builds)
```

## 10. Implementation Order

| Phase | Content | Estimated changes |
|------|------|------|
| 1 | `core/scip/registry.go` + `toolchain.go` (registry + auto-download, reuse core/installer) | ~200 lines |
| 2 | `core/scip/detect.go` + `runner.go` (language detection + commit cache + lock) | ~200 lines |
| 3 | `core/scip/index.go` + `index_test.go` (SCIP load + 4 query APIs) | ~350 lines |
| 4 | `core/git/agent.go` integration (3 new tools + read_function upgrade + fallback) | ~200 lines |
| 5 | `scip/` CLI + `git-config.json` scip config + review integration (`--no-scip`/`--refresh-scip`) | ~250 lines |
| 6 | README docs + `tasks.md` update + full project compile/test/lint | — |
| 7 | Rust support: add rust to registry + data-driven `generate()` + bare `.gz` extraction | ~80 lines |
| 8 | Java support: `AssetName` download by name + registry enable + `generate()` adapt + auto-build/fail-fast | ~120 lines |

## 11. Risks and Trade-offs

| Risk | Mitigation |
|------|------|
| Indexer distribution methods vary (npm/pip/gem) | Registry `InstallMethod` abstraction; v1 only commits to GitHub release (Go/Rust/Java/Clang); TS via npx falls to v2 |
| scip-go runtime needs `go` in PATH | `Requires` field + preflight check, fallback hint when missing |
| rust-analyzer scip runtime needs `cargo`+`rustc` | `Requires: ["cargo","rustc"]` + preflight check; rust-analyzer itself auto-downloaded |
| scip-java asset name has no OS/arch | `Indexer.AssetName` download-by-name branch, bypasses `core/installer`'s OS/arch filtering |
| scip-java runtime needs JDK 17+ + build tools | `Requires: ["java"]` + build tool/project wrapper detection; skip Java and hint when missing |
| scip-java executes full build per index (minutes, with side effects) | Review **auto-builds** (builds on no index/stale, fail-fast), prints reason line + spinner; `--refresh-scip` forces rebuild; build output goes to temp file |
| First review download + index latency | Commit cache reuse; verbose prints progress; `--refresh-scip` manual control |
| Large repo dirty repeated re-indexing | Per-commit cache + only regenerate working index when dirty; when dirty, check source file mtime freshness to decide whether to rebuild |
| Line number ↔ offset conversion errors | Cache line start offset table per file, unit tests cover multi-byte characters (Chinese comments) |
| Overlap with `tree-sitter-wasm-plan.md` | SCIP plan supersedes Option A/C in that document (`find_definition` goals), after completion mark as implemented by SCIP in that document |

## 12. Implementation Progress

| Phase | Content | Status |
|------|------|------|
| 1 | registry + toolchain | ✅ |
| 2 | detect + runner | ✅ |
| 3 | index query API + tests | ✅ |
| 4 | agent tool integration | ✅ |
| 5 | CLI + config | ✅ |
| 6 | docs + verification | ✅ |
| 7 | Rust support (registry + data-driven generate + bare `.gz` extraction) | ✅ |
| 8 | Java support (AssetName download + buildArgs + temp output file + FailHard + spinner) | ✅ |
| 9 | Version configurability + `mu scip update` (`AssetName` → `AssetNameTemplate`, `review.scip.versions` override, `ResolveVersion` config > pin > latest, `update`/`--release`/`--scip-version`, `LatestTag` parsing) | ✅ |
