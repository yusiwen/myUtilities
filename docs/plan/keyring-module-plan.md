# Keyring Credential Storage Module Plan

> **Status: 📋 Planned, paused (paused).** API keys are still stored in plaintext in config files.

## Background

Currently, API keys for `ask`, `git commit/review`, and `budget` are stored in plaintext in `~/.config/mu/*-config.json`.
Goal: Provide `mu secret` subcommands to store keys in the OS-native keyring, and have each module
prioritize reading from the keyring when resolving API keys (`--flag/env → config file → keyring`),
so config files no longer need to store plaintext.

## Dependencies

`github.com/zalando/go-keyring` (pure Go, no CGo, maintain cross-platform `CGO_ENABLED=0` builds)

| Platform | Backend |
|------|------|
| macOS | Keychain (`/usr/bin/security`) |
| Linux/*BSD | Secret Service protocol (D-Bus, GNOME Keyring/KWallet) |
| Windows | Credential Manager (target = `service:user`) |

## Storage Model

go-keyring is a two-dimensional key/value store: `(service, user) → value`.

```go
keyring.Set(service string, user string, password string) error
keyring.Get(service string, user string) (string, error)
keyring.Delete(service string, user string) error
```

## Directory Structure

```
internal/secret/
  ├── options.go     set / get / list / rm subcommand flags
  ├── command.go     CLI entry, logic delegated to internal/core/secret
  └── set.go         Optional: config.Register(&secretSetter{}) (if merged into mu set)

internal/core/secret/
  ├── keyring.go     Set/Get/Delete wrappers (with error mapping, Linux headless fallback)
  └── index.go       list index (if index approach is adopted)
```

## CLI Interface

```
mu secret set <key> <value>      # e.g., mu secret set ask.api_key sk-xxx / budget.deepseek sk
mu secret get <key>              # output plaintext (optional --quiet)
mu secret list                   # list stored key names
mu secret rm <key>               # delete
mu secret rm --all               # clear all
```

`<key>` naming convention: `<module>.<field>` (`ask.api_key`, `git.default`, `budget.deepseek`,
`budget.openrouter`, `es.password`...), the command layer handles `key → (service,user)` mapping,
backing into the OS keyring.

## Integration: Resolution Chain

Unified priority: **`--flag` / env → config file → keyring** (when config is empty string, continue checking keyring).

| Module | Current resolution point | Change |
|------|-----------|------|
| `ask` | `internal/ask/command.go:155` (`cfg.APIKey == ""` error) | Check keyring when empty |
| `git commit/review` | `internal/git/review.go:71`, `commit.go:66` (provider.APIKey) | Check keyring when empty |
| `budget` | `internal/core/budget/config.go:60` `ResolveAPIKey` | Check keyring when empty |

## Error Handling (Linux headless environments)

- When D-Bus Secret Service is unavailable, `keyring.Get` returns error → fallback: revert to config file + print hint
  (prompt to install `libsecret-tools` / `gnome-keyring` / unlock keyring)
- `mu secret set` provides actionable error messages when keyring is unavailable

## Testing

- `internal/core/secret` unit tests use `keyring.MockInit()` (go-keyring provides in-memory implementation)
- Coverage: Set/Get/Delete round-trip, key naming mapping, keyring unavailable fallback path

## Implementation Steps

| Step | Content |
|------|------|
| 1 | `go get github.com/zalando/go-keyring` + verify no CGO |
| 2 | `internal/core/secret/keyring.go` + `index.go` |
| 3 | `mu secret` CLI (options.go/command.go) register in `cmd/mu/myutilities.go` |
| 4 | Unit tests (MockInit) |
| 5 | Integrate ask / git / budget resolution chain |
| 6 | Documentation: README, AGENTS.md (config conventions add keyring description), ROADMAP #6 → Done |
| 7 | Full project build/vet/test verification |

## Pending Decisions

### D1. `(service, user)` key naming

- **Option A (Recommended) — Module separation:** service = `mu-ask` / `mu-budget` / `mu-git`, user = `api_key` or provider name.
  Keyring GUI (Seahorse/Keychain Access) shows each module as a separate entry, easier to identify and manage.
- **Option B — Single service:** service = `mu`, user = `<module>.<field>`. More compact, but all entries
  are mixed under the same service in GUI, deletion/troubleshooting relies on user name distinction.

### D2. `mu secret list` implementation (go-keyring does not support enumerating entries)

- **Option 1 (Recommended) — Maintain index file:** Record `key → (service,user)`
  mapping in `~/.config/mu/secret-index.json`, sync add/delete on `set`/`rm`. `list` reads index directly. Drawback: index may be inconsistent with OS keyring (when user manually
  adds/deletes via `secret-tool`); needs to handle index corruption/absence.
- **Option 2 — Do not maintain index:** `list` relies on platform tools (macOS `security dump-keychain` filter / Linux
  `secret-tool search`), or simply does not support list. Simpler implementation, but cross-platform output format is inconsistent and parsing is fragile.
- **Option 3 — Support known keys only:** `list` outputs a preset set of known keys (the full set of optional keys defined in config files),
  marking which ones exist. No index needed, but does not reflect user-defined keys.

### D3. Whether to merge into `mu set`

- **Option A (Recommended) — Independent `mu secret` command:** Consistent with original ROADMAP plan, clear responsibilities.
- **Option B — Merge into `mu set`:** Reuse `ModuleSetter` registration mechanism, but the semantics (store in keyring rather than config file)
  differ from existing setters, more invasive.
