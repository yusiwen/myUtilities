# Codebase Restructure Plan

This document records the detailed plan for code organization and structural optimization of the myUtilities project (corresponding to tasks 6-11 in `tasks.md`).

## Current Issues List

| # | Issue | Severity | Description |
|---|-------|----------|-------------|
| A | Package name conflicts | 🔴 High | `core/crypto` vs `crypto/`, `core/git` vs `git/`, `core/runner` vs `runner/`, `core/proxy` vs `proxy/` — 4 groups of same-name packages, must use import aliases |
| B | Version in main package | 🟡 Medium | `Version` / `CommitSHA` / `BuildTime` in `package main`, other packages cannot reference directly |
| C | PascalCase filenames | 🟡 Medium | `core/proxy/Proxy.go` etc. 6 files violate Go filename conventions |
| D | Messy project root | 🟡 Medium | `main.go` `myutilities.go` `version.go` 3 entry files at root, not organized per standard layout |
| E | CLI command name ≠ package name | 🟢 Low | `name:"install"` → `package installer`, `name:"run"` → `package runner`, `name:"jar"` → `package jarinfo` |
| F | core/net files split | 🟢 Low | `interface.go` vs `interfaces.go`, unclear content boundaries |
| G | Only 2 TODOs | 🟢 Low | Two TODOs in `installer/command.go` for deb/rpm and powershell |
| H | 0 commented-out code lines | 🟢 Low | Already clean, no special cleanup needed |

## Phase 1 — Quick Wins

Estimated total effort: **~45 minutes**. Three tasks are independent and can be executed in parallel. Low risk.

### 1-① Extract version info to independent package

**Goal:** Solve B, allowing all packages to reference version info directly.

**Steps:**

1. Create `internal/core/version/version.go`:
   ```go
   package version

   var (
       Version   = "unknown version"
       BuildTime = "unknown time"
       CommitSHA = ""
   )
   ```

2. Delete root `version.go` (original `package main`)

3. Modify `main.go`:
   - Add import `v "github.com/yusiwen/myUtilities/internal/core/version"`
   - Change all `Version` / `CommitSHA` / `BuildTime` to `v.Version` / `v.CommitSHA` / `v.BuildTime`

4. Modify `Makefile`:
   - `main.Version` → `github.com/yusiwen/myUtilities/internal/core/version.Version`
   - `main.CommitSHA` → `github.com/yusiwen/myUtilities/internal/core/version.CommitSHA`
   - `main.BuildTime` → `github.com/yusiwen/myUtilities/internal/core/version.BuildTime`

5. Verify: `make build && ./bin/mu --version`

**Files involved:** `internal/core/version/version.go` (new), `main.go` (modify), `Makefile` (modify)

---

### 1-② Fix PascalCase filenames

**Goal:** Solve C, rename 6 files to Go convention snake_case.

**Steps:**

Pure `git mv`, no need to change `package` declarations in files (Go groups by directory name).

| Current path | Change to |
|----------|------|
| `core/proxy/Proxy.go` | `core/proxy/proxy.go` |
| `core/proxy/db/DBProxy.go` | `core/proxy/db/dbproxy.go` |
| `core/watcher/GitWatcher.go` | `core/watcher/gitwatcher.go` |
| `core/watcher/FileWatcher.go` | `core/watcher/filewatcher.go` |
| `core/runner/CommandRunner.go` | `core/runner/commandrunner.go` |
| `mock/oauth/AuthServer.go` | `mock/oauth/authserver.go` |

**Check needed:** Whether other files import these packages (Go imports by package name, not filename, so imports generally don't need changes). But confirm no files use `import "./..."` relative paths.

**Files involved:** 6 files renamed

---

### 1-③ Clean up TODOs

**Goal:** Solve G, handle the 2 TODOs in `installer/command.go`.

**Steps:**

1. Create issues on GitHub for each feature (if not already)
2. Replace TODOs with standard comments pointing to issues:
   ```go
   // TODO(#123): deb,rpm etc
   // TODO(#124): powershell
   ```

Or if deciding not to do them in the short term, simply delete the TODO lines.

**Files involved:** `installer/command.go` (modify)

---

## Phase 2 — Package Name Conflicts

Estimated total effort: **~2 hours**. Medium risk, needs verification build pass by pass.

### 2-④ Resolve 4 groups of package name conflicts

**Goal:** Solve A, eliminate all import alias requirements.

**Background:** 4 groups of same-name packages:

| Package name | Command path | Core path | Current alias approach |
|------|---------|-----------|----------------|
| `git` | `git/` | `core/git/` | `coregit "..."` |
| `runner` | `runner/` | `core/runner/` | `corerunner "..."` |
| `proxy` | `proxy/` | `core/proxy/` | (no explicit alias, indirectly used via call chain) |
| `crypto` | `crypto/` | `core/crypto/` | `corecrypto "..."` |

#### Option A (Recommended): Rename core packages

Rename on the `core/` side, without affecting CLI user interface:

| Current | Change to |
|------|------|
| `core/git` → `package gitcore` | directory `core/git/` → `core/gitcore/` |
| `core/runner` → `package cmdexec` | directory `core/runner/` → `core/cmdexec/` |
| `core/proxy` → `package proxycore` | directory `core/proxy/` → `core/proxycore/` |
| `core/crypto` → keep `package crypto` | directory unchanged (distinguished by upper-level alias) |

Affected import updates:

| Old import | New import | Files involved |
|-----------|----------|---------|
| `core/git` | `core/gitcore` | `git/commit.go`, `git/ignore.go` |
| `core/runner` | `core/cmdexec` | `runner/runner.go`, `runner/options.go` |
| `core/proxy` | `core/proxycore` | `proxy/dbproxy.go`, `core/proxy/db/DBProxy.go` (`core/proxy` itself needs renaming) |

Selection rationale:
- ✅ Does not change CLI command names (user interface unchanged)
- ✅ Does not change command examples in documentation
- ✅ Does not change Makefile build targets
- ✅ Does not change frontend embed paths
- ❌ Needs to update imports in 6 files

#### Option B (Alternative): Rename command packages

| Current | Change to | CLI command name |
|------|------|-----------|
| `package git` in `git/` | `package gittool` in `gittool/` | `name:"git"` unchanged |
| `package runner` in `runner/` | `package runcmd` in `runcmd/` | `name:"run"` unchanged |
| `package proxy` in `proxy/` | `package proxysrv` in `proxysrv/` | `name:"proxy"` unchanged |
| `package crypto` in `crypto/` | `package cryptotool` in `cryptotool/` | `name:"crypto"` unchanged |

Selection rationale:
- ✅ Core stays clean, no renaming
- ❌ Makefile paths must change (`go build ./crypto/` → `./cryptotool/`)
- ❌ Frontend embed relative paths must change (`crypto/frontend/dist` → `cryptotool/frontend/dist`)
- ❌ Directory names inconsistent with CLI command names, increasing confusion

**Conclusion: Option A recommended.**

---

## Phase 3 — Standard Project Layout

Estimated total effort: **~1 day**. High risk, needs careful evaluation.

### 3-⑤ Migrate to cmd/ + internal/

> ✅ **Completed** (2026-08-03). Key decisions:
>
> | Decision point | Conclusion |
> > |--------|------|
> > | Location for core business logic packages | `internal/core/` (pure CLI tool, all packages externally non-importable) |
> > | Resolve package name conflicts simultaneously | Only directory migration, keep 4 groups of same-name package aliases (`corecrypto`/`coregit`/`corerunner` etc.) |
> > | `shared/frontend/` location | Move to `web/shared/frontend/` |
> > | Commit split | 3 commits: ① Move + imports ② Build system ③ Documentation |
> > | Makefile `default:` no version injection | Keep as is (just change `.` to `./cmd/mu`), don't attempt to reuse GOBUILD |
> >
> > Actual scale correction: ~180 .go files, 23 command packages, 23 core sub-packages, 12 frontend modules, 42 files / 103 import rewrites.

**Goal:** Solve D, adjust the project to standard Go project layout.

**Actual implemented layout:**

```
myUtilities/                     (go.mod)
├── cmd/
│   └── mu/
│       ├── main.go              (package main, entry point only + kong.Parse)
│       └── myutilities.go       (package main, CLI struct)
├── internal/                    (externally non-importable)
│   ├── gateway/  wol/  es/  ...
│   ├── crypto/  diff/  k8s/  ...
│   └── core/                    (internal/core/)
│       ├── crypto/  git/  runner/  proxy/  ...
│       └── net/  openai/  store/  watcher/
├── web/                          (frontend resources)
│   └── shared/frontend/
├── Makefile
└── README.md
```

**Impact assessment (actual):**

| Aspect | Impact |
|------|------|
| File moves | ~180 Go files, 12 frontend directories (including dist, `git mv` OS rename moves together), web/shared |
| Import paths | All `github.com/yusiwen/myUtilities/X` → `.../internal/X`, 103 locations (including deep core paths and test files) |
| `//go:embed` | Unchanged (paths relative to file itself, files follow directories; includes `mock/oauth` templates + static) |
| Makefile | `go build -o bin/mu .` → `./cmd/mu/` (GOBUILD + default two locations) |
| ldflags | Unchanged (`-X main.*`, main package still in cmd/mu) |
| `install.sh` | Unchanged (only references binaries) |
| CLI commands | Unchanged |
| Web UI routes | Unchanged |
| User-visible behavior | No changes |

**Decision analysis:** This project is a CLI tool + Web UI, not designed to be imported by external users. The protection value of `internal/` is limited. The benefit of `cmd/mu/` is clear entry point, but the cost is ~180 files of import path changes and regression testing.

**Recommendation:** Evaluate whether Phase 3 is still needed after Phase 1 + 2. If Phase 2 (resolving package name conflicts) significantly improves code structure, Phase 3 can be downgraded or skipped.

---

## Phase 4 — Naming Cleanup

Estimated total effort: **~1 hour**. Low risk.

### 4-⑥ Unify naming conventions

**Goal:** Solve E + F + H, complete remaining naming and comment issues.

**Steps:**

1. **Fix CLI command name vs package name inconsistency**
   These are determined by Kong's `name:` tag, package names can keep internal naming without aligning to CLI names. But consider adding explanatory comments.

2. **Merge or re-divide `core/net/`**
   `interface.go` and `interfaces.go` can be merged, or renamed to `netiface.go` + `wol.go` etc. for more explicit names.

3. **Unify comment language to English**
   Clean up Chinese comments in `core/proxy/Proxy.go`, `core/proxy/db/DBProxy.go`, `core/watcher/FileWatcher.go`, `mock/oauth/AuthServer.go`, change to English.

---

## Work Summary

| Phase | Content | Files involved | Estimated effort | Risk |
|-------|---------|---------------|-----------------|------|
| 1-① | Independent version package | 3-4 | ~30min | 🟢 Low |
| 1-② | Fix filenames | 6 | ~10min | 🟢 Low |
| 1-③ | Clean TODOs | 1 | ~5min | 🟢 Low |
| 2-④ | Package name conflicts | 6-8 | ~2h | 🟡 Medium |
| 3-⑤ | Standard layout | ~180 | ~1d | 🔴 High |
| 4-⑥ | Naming cleanup | 5-10 | ~1h | 🟢 Low |

## Recommended Execution Order

1. **Phase 1** — Quick Wins, do immediately, low risk
2. **Phase 2** — Package name conflicts, resolve the biggest pain point
3. **Phase 3** — Standard layout ✅ Completed
4. **Phase 4** — Naming cleanup, finish last
