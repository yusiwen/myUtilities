# WireGuard Configuration Management Module Plan (`mu wg`)

> **Status: 📋 Planned, pending implementation.** Design has been confirmed with the user, can be implemented per this plan.

## Background

Need a small module to manage WireGuard configuration files:

1. Read the current configuration file, default `/etc/wireguard/wg0.conf`, can be specified via parameter.
2. List all current peers information.
3. Maintain the mapping of "peer ↔ name/note". This name/note is not supported in the official configuration file, so it needs to be managed separately.

### Core Challenge

WireGuard's official configuration (wg-quick format) only has `PublicKey`, `Endpoint`, `AllowedIPs`,
`PersistentKeepalive`, `PresharedKey` and other fields, **no stable "name" field**.
Therefore, peer metadata must be stored externally, and **use `PublicKey` as the unique key** (IP changes, key doesn't).
Additionally, official tools (`wg setconf` / `wg syncconf`) strip comments when re-serializing the configuration,
so the "write name into conf comment" approach is unreliable.

### Confirmed Trade-offs (User decided)

| Decision point | Choice |
|--------|------|
| Metadata storage location | Sidecar JSON (`<conf>.meta.json` placed next to conf) |
| Feature scope | Only "metadata management + list", does not read/write conf files themselves |
| Web UI | Pure CLI, no serve/frontend |
| Real-time status | Optional `--live` (calls `wg show`, requires root) |

## Storage Model

### Sidecar Metadata File

Path: `<conf path>.meta.json`, e.g. `/etc/wireguard/wg0.conf.meta.json`.

```json
{
  "version": 1,
  "peers": {
    "BASE64PUBLICKEY1=": { "name": "home-nas", "note": "Home NAS, via NAT exit" },
    "BASE64PUBLICKEY2=": { "name": "phone",   "note": "" }
  }
}
```

- Key is the peer's `PublicKey` (44-character base64, unique and stable).
- File permission `0600`.
- Advantages: metadata follows configuration, backup/migration naturally consistent; no global naming conflicts; write permission requirements same as changing conf (both require root), no permission gap.

### conf parsing

Write a lightweight INI parser, **read-only, never write back** the conf file. Features:

- Keys are case-insensitive (WireGuard parser is case-insensitive).
- `#` comments skip entire lines; values split at first `=`.
- Parse by `[Interface]` / `[Peer]` sections, preserve section order.
- `AllowedIPs` is comma-separated multiple addresses.

## Directory Structure

```
internal/wg/            # CLI wrapper (kong subcommands)
  ├── options.go        # Options: list / rename / note / prune subcommand flags
  ├── command.go        # list implementation + resolveConfPath() (--config/--interface/env/default)
  └── meta.go           # rename / note / prune implementation + peer lookup (name exact / pubkey prefix)

internal/core/wg/       # Business logic, independently testable
  ├── conf.go           # Parse(data) → *Config{Interface, []Peer}
  ├── meta.go           # MetaStore: LoadMeta / SaveMeta (0600)
  ├── join.go           # ListPeers(conf, meta) → []PeerRow (join + mark unnamed/orphan)
  └── live.go           # ShowLive(iface) → parse `wg show <iface> dump`, merge by pubkey
```

Register: `cmd/mu/myutilities.go` add `Wg wg.Options cmd:"" name:"wg" help:"WireGuard config management."`

## CLI Interface

```
mu wg list    [--config PATH] [--interface wg0] [--live] [--json]
mu wg rename  [--config PATH] <peer> <new-name>
mu wg note    [--config PATH] <peer> <note>
mu wg prune   [--config PATH]        # clean up pubkeys in meta that have disappeared from conf
```

### conf path resolution priority

`--config <path>` > `--interface <name>` (derive `/etc/wireguard/<name>.conf`) >
env `MU_WG_CONFIG` > default `/etc/wireguard/wg0.conf`.

### `<peer>` lookup rules

1. Name exact match (meta's `name`) → direct hit.
2. Otherwise, match by pubkey prefix (≥6 characters), only if uniquely matched.
3. If cannot determine uniquely, list candidates (similar names/prefixes) instead of erroring.

### list output

tabwriter columns: `NAME | PUBLIC KEY (short) | ENDPOINT | ALLOWED IPS | KEEPALIVE | NOTE`.
`--live` appends `HANDSHAKE | RX | TX`. `--json` outputs structured data for scripting.

## Edge Cases

- Unnamed peers display pubkey first 8 characters.
- Pubkeys in meta but disappeared from conf (orphan) → warn on list, clean up with `prune`.
- Peer not found → list candidates.
- `--live` fails (`wg` not installed / not root / interface not found) → non-fatal warning, fall back to pure conf output.
- Do not write back to the conf file itself (v1 scope confirmed).

## Testing

- `internal/core/wg/conf_test.go` — Parse: multiple peers, case-insensitive, comments, empty values, AllowedIPs comma-separated.
- `internal/core/wg/meta_test.go` — Load/Save round-trip, corrupted file fault tolerance.
- `internal/core/wg/live_test.go` — `wg show <iface> dump` output parsing.
- `internal/core/wg/join_test.go` — Unnamed / orphan / normal join.

## Implementation Steps

| Step | Content |
|------|------|
| 1 | `internal/core/wg/conf.go` + `conf_test.go` |
| 2 | `internal/core/wg/meta.go` + `meta_test.go` |
| 3 | `internal/core/wg/join.go` + `join_test.go` |
| 4 | `internal/core/wg/live.go` + `live_test.go` |
| 5 | `internal/wg/options.go` / `command.go` / `meta.go` (CLI) |
| 6 | Register `cmd/mu/myutilities.go`, README.md command list, docs/wg.md, CODEBASE.md |
| 7 | Full project build / vet / fmt / test verification |

## Pending Decisions (Future Optional)

### D1. Whether to merge into `mu set wg`

- **Option A (Recommended) — Do not merge:** conf path via flag + env is sufficient, keep the module minimal.
- **Option B — Merge:** `config.Register(&wgSetter{})` provides `mu set wg --default-config`,
  persist default conf path. Consistent with project `<module>-config.json` conventions, but not needed for v1.

### D2. Whether to support conf add/delete/modify

- **Option A (Recommended) — Do not do:** Read-only conf, avoid security rewrite (preserve comments/format) complexity and risk.
- **Option B — Add add/remove/set:** Needs secure rewriter, high risk, leave for future versions.
