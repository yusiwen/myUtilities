# Tree-sitter WASM Integration Plan

## Background

`mu git review` agent currently has a `search_code` tool that uses `git grep`
for text-based pattern matching. This cannot distinguish between **call sites**
and **definitions**, leading the LLM to waste rounds searching for symbols
that exist but whose definitions don't match the grep pattern.

A proposed tool `find_definition(symbol, file?)` would resolve this by using
AST-level parsing to locate exact symbol definitions.

## Options

### Option A: `go/ast` (stdlib) + text fallback (Recommended for v1)

| Language | Engine | Dependency |
|----------|--------|------------|
| Go | `go/parser` + `go/ast` + `go/types` | **Zero** (Go stdlib) |
| All others | Regex heuristic (`func\s+\w+`, `def\s+\w+`, etc.) | Zero |

**Pros:**
- Zero new dependencies
- No CGo, no WASM — full cross-compilation compatibility (`CGO_ENABLED=0`)
- Go symbol resolution is precise (type-level, not just syntax)
- ~100 lines of Go code

**Cons:**
- Non-Go languages get only text-based fallback (same as current `search_code`)
- `go/ast` needs the file to be parseable (valid Go syntax)

### Option B: CGo + smacker/go-tree-sitter (Full coverage, breaks cross-compile)

**Pros:**
- Mature library, widely used
- 100+ languages supported with native parsers
- Precise AST-level queries for all languages

**Cons:**
- Requires `CGO_ENABLED=1` — incompatible with existing Makefile
- Each language parser is a C library (~200KB-2MB each, static linked or .so)
- Cross-compilation for 7+ platforms becomes impractical
- Binary size increases significantly

Makes the build process significantly more complex. Not recommended unless
cross-compilation is abandoned.

### Option C: WASM + wazero (Full coverage, no CGo)

Loads precompiled tree-sitter grammar `.wasm` files at runtime using
`github.com/tetratelabs/wazero` (pure Go WASM runtime, no CGo).

#### How it works

```
Runtime:

1. detectLanguage(file) — check go.mod / Cargo.toml / package.json / pyproject.toml
2. loadGrammar(lang):
   a. Check ~/.cache/mu/grammars/<lang>.wasm
   b. If missing, download from tree-sitter release or npm
   c. Use wazero to instantiate the WASM module
3. Parse file → AST → query definition
```

#### WASM Sources

Tree-sitter publishes precompiled WASM grammars for all languages:

| Source | URL Pattern | Format |
|--------|-----------|--------|
| **Official tree-sitter releases** | `https://github.com/tree-sitter/tree-sitter-<lang>/releases` | `.wasm` |
| **npm packages** | `npm install tree-sitter-<lang>-wasm` | `.wasm` in package |

Languages available: Go, Python, TypeScript, JavaScript, Rust, Java, C, C++,
Ruby, PHP, Swift, Kotlin, 100+ total.

#### Key Uncertainty

The official WASM grammars are compiled for **browser** environments (targeting
`web-tree-sitter` JavaScript runtime). Loading them in `wazero` (pure Go WASM
runtime) requires:

- WASI interface support — wazero provides this
- Tree-sitter-specific host function imports (memory, env, etc.) — **unknown**
  whether these are compatible with a Go WASM runtime without a C bridging layer.

The CGo-based `smacker/go-tree-sitter` loads `.so` files, not `.wasm` files.
Neither `smacker/go-tree-sitter` nor `tree-sitter/go-tree-sitter` currently
support loading WASM grammars through a Go-native WASM runtime.

#### Required Investigation

Before committing to Option C, verify:

```
1. Can wazero load a tree-sitter grammar.wasm without host function stubs?
   Example: tree-sitter-go.wasm from: npm install tree-sitter-go-wasm

2. If not, can we write a thin C bridge with emscripten that re-exports
   the grammar as a WASI-compatible module?

3. If neither works, can we strip the CGo dependency in smacker/go-tree-sitter
   to only use its query API while loading grammars through wazero?
```

**Recommendation:** Spend 1-2 hours on investigation before deciding. If
Option C proves viable, it's the ideal solution. If not, start with Option A
and reconsider when user demand for non-Go symbol resolution emerges.

#### Implementation Estimate (if viable)

| Package | Lines | Description |
|---------|-------|-------------|
| `core/wasm/` | ~80 | wazero runtime, grammar download/cache |
| `core/ast/` | ~120 | language detection, parse, query wrapper |
| `git/review_agent.go` | ~10 | `find_definition` tool registration |

Total: ~210 lines, zero CGo, cross-compile safe.

## Recommendation (Current)

**Start with Option A** (`go/ast` + text fallback). It solves the primary
use case (Go code, which is the project's own language) with zero additional
complexity. If non-Go symbol resolution becomes a frequent request, revisit
Option C investigation.

### Option A Implementation Plan

```
git/review_agent.go — Add tool `find_definition(symbol, file?)`:
  1. detectLanguage(file) → "go" | "other"
  2. If "go":
     a. os.ReadFile(file)
     b. go/parser.ParseFile → go/ast inspects for function/type/variable defs
     c. Return enclosing definition + surrounding lines
  3. If "other":
     a. Run git grep with ^\\s*func\\s+\\b<symbol>\\b or similar
     b. Return matching line + context
```
