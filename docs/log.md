# log — Log tailer and filter

Tail and filter log files from the terminal. Supports plain-text and
JSON-line formats with automatic level detection, level filtering, time
window filtering, and regex matching.

```bash
# Show the last 100 lines
mu log app.log

# Show last 50 lines
mu log -n 50 app.log

# Show all lines
mu log -n 0 app.log

# Follow log file in real time (Ctrl-C to stop)
mu log -f app.log

# Show only errors and above
mu log -l error app.log

# Show only warnings and above
mu log -l warn app.log

# Show only entries from the last 5 minutes
mu log --since 5m app.log

# Grep for lines containing "timeout"
mu log -g "timeout" app.log

# Combine filters
mu log -f -l error -g "db|database" app.log

# Multiple files
mu log -n 20 /var/log/app/access.log /var/log/app/error.log

# JSON-line logs (level auto-detected from "level"/"severity"/"lvl" fields)
mu log -l error app.json.log
```

## Flags

| Flag | Description |
|---|---|
| `-f`, `--follow` | Continually watch and print new log lines (like `tail -f`) |
| `-l`, `--level` | Minimum log level to show: `debug`, `info`, `warn`, `error`, `fatal` (default: `debug` = show all) |
| `-s`, `--since` | Only show entries newer than this duration (e.g. `5m`, `1h`, `2h30m`) |
| `-g`, `--grep` | Only show lines matching this regex pattern |
| `-n`, `--lines` | Number of lines to show initially (default: 100, `0` = all lines) |
| `-C`, `--no-color` | Disable colored output (also respects `NO_COLOR` env var) |

## Behavior

- **Multi-file** — when multiple files are given, a `── path ──` separator
  is printed before each file's lines.
- **Level detection** — for plain-text lines, the level keyword is detected
  anywhere in the line (`debug`, `info`, `warn`/`warning`, `error`,
  `fatal`/`critical`). For JSON lines, the `level`, `severity`, `lvl`, or
  `loglevel` field is used.
- **Level filtering** — when a minimum level is set, lines whose detected
  level is below the minimum are suppressed. Lines with no detectable level
  are always shown.
- **Time filtering** — when `--since` is set, entries with a parsed timestamp
  older than the duration are suppressed. Lines without a detectable timestamp
  are always shown.
- **Grep** — a regular expression matched against the raw line; non-matching
  lines are suppressed.
- **Colors** — plain-text lines are colored by level: DEBUG=faint, INFO=green,
  WARN=yellow, ERROR=red, FATAL=red+bold. JSON lines are printed as-is.
  Colors are disabled when output is piped or when `NO_COLOR`/`-C` is set.
- **Follow mode** — polls files every 500 ms. Detects file truncation/rotation
  and resets the read offset. Press Ctrl-C to stop.
