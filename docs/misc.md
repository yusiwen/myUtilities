# misc — Miscellaneous tools

JSON formatter/validator, UUID generator, timestamp converter, hash calculator,
and a BitTorrent trackers list fetcher (web UI only).
Supports both CLI and web UI.

```bash
# UUID generation
mu misc uuid
mu misc uuid 5

# JSON operations
mu misc json format '{"a":1}'
mu misc json validate '{"a":1}'
mu misc json minify '{ "a": 1 }'

# Timestamp conversion
mu misc timestamp              # current Unix time
mu misc ts "2025-01-01"        # date → Unix
mu misc ts 1735689600          # Unix → human date

# Hash computation
mu misc hash sha256 "hello"
mu misc hash md5 "hello"
mu misc hash sha512 -f file.txt

# Serve web UI (standalone)
mu misc serve --port 8090
```

The web UI provides:
- **JSON** tab — format, validate, and minify JSON
- **UUID** tab — generate UUID v4, single or batch
- **Timestamp** tab — auto-detect Unix timestamp or ISO date, real-time conversion
- **Hash** tab — compute MD5, SHA-256, SHA-512 hashes with file upload support
- **Trackers** tab — fetch and refresh the best BitTorrent trackers list from the
  [ngosang/trackerslist](https://github.com/ngosang/trackerslist) project
