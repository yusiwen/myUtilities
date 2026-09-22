# Multi-threaded Resumable Download Plan (`mu network download`)

> **Status: ✅ Implemented.** Scope: CLI only; block-queue segmentation, automatic resume,
> single-line aggregate progress (hand-rolled ANSI), plus `--sha256`, `--limit-rate`,
> `--proxy`, `--http1`.
>
> Implementation: `internal/core/downloader/` (engine + tests) and
> `internal/network/download.go` / `progress.go` (CLI + renderer); user docs in
> `docs/network.md`.
>
> Deviations from the draft below:
>
> - `--verbose` has no short form: `-v` is the global `--version` flag and Kong
>   rejects the duplicate.
> - Added `--no-preallocate` (preallocation is on by default, with a graceful
>   fallback when `Truncate` fails).
> - Single-stream mode (`--threads 1` on a range-capable server) still uses the
>   block machinery, so it keeps resume support; only servers that ignore `Range`
>   fall back to a truly non-resumable sequential download.
> - Fixed a pre-existing `mu network` defect found while testing: Kong runs every
>   `Run` method on the command path, so the parent `Options.Run()` error made all
>   subcommands exit `1` after doing their work. The parent now returns early when
>   a subcommand was selected.


## Background

`mu network` currently covers DNS, DIG, WHOIS, TLS cert inspection, a curl-like HTTP
client, and port scanning. It has no bulk file transfer capability: `mu network http -o`
does a single-connection, non-resumable body dump.

Goal: add `mu network download <url>` with:

1. **Multi-connection** transfer (HTTP range requests, N parallel workers).
2. **Resume** (breakpoint continuation) that survives process restart and Ctrl-C.
3. **Live display** of active worker count, aggregate speed, progress bar, ETA.

## Non-goals (v1)

- FTP / SFTP / BitTorrent / magnet links.
- HLS/DASH segment assembly, browser cookie-jar login flows.
- Archive auto-extraction (possible v2: reuse `internal/core/fleet` `ExtractArchive`).
- Uploading.

## Directory Layout

```
internal/core/downloader/        # business logic, no CLI/terminal dependency
  downloader.go   # Download(ctx, Options) (*Result, error) — orchestration + Run lifecycle
  probe.go        # Probe(ctx, url) → RemoteInfo{Size, AcceptRanges, ETag, LastModified, Filename, FinalURL}
  plan.go         # block planning (block size auto-tune, block count, bitmap)
  worker.go       # per-block fetch loop: range request, stall timeout, retry/backoff
  state.go        # sidecar state file load/save (atomic rename, throttled flush)
  writer.go       # preallocate, concurrent WriteAt, final fsync + rename
  stats.go        # atomic counters: total bytes, per-connection bytes, speed sampling
  errors.go       # typed errors (ErrNoRange, ErrResumeMismatch, ErrChecksum, ...)
  downloader_test.go / probe_test.go / state_test.go / resume_test.go

internal/network/download.go     # Kong CLI wrapper: DownloadOptions + Run()
internal/network/progress.go     # terminal renderer (TTY) + plain/quiet fallback
```

Register in `internal/network/command.go`:

```go
Download DownloadOptions `cmd:"" name:"download" help:"Multi-threaded resumable file download."`
```

Rationale: CLI wrappers stay thin (flag parsing, presentation, user interaction);
all transfer logic lives in `internal/core/downloader` so it is unit-testable with
`httptest` and reusable (e.g. later by `mu network http -o` or installer paths).

## Transfer Design

### Architecture

```
              Probe (HEAD → fallback GET Range:0-0)
                          │
        ┌─────────────────┴──────────────────┐
        │ range supported                    │ no range support
        ▼                                    ▼
  block queue + bitmap                 single-stream mode
  (N workers, WriteAt)                 (1 goroutine, sequential write)
        │                                    │
        └──────────────┬─────────────────────┘
                       ▼
        stats (atomic)  →  progress renderer (10 Hz ticker)
                       ▼
        state flush (1 Hz)  →  <output>.mu-dl.json
                       ▼
        verify (size / optional sha256) → fsync → rename .part → final
```

### Segmentation: block queue with a bitmap (recommended)

Instead of splitting the file into N fixed contiguous chunks, split it into
fixed-size **blocks** (default 8 MiB) held in a shared queue. Workers pull the next
unfinished block, request that byte range, `WriteAt` it, and mark the block done.

| Aspect | Block queue + bitmap (A) | N fixed contiguous chunks (B) |
|---|---|---|
| Load balancing | Good: a slow connection only delays one block | Bad: slowest chunk gates the whole file |
| Resume granularity | Per block, cheap and precise | Per chunk offset, also precise |
| Request count | Higher (size / block size) | Exactly N |
| Progress accounting | `doneBlocks * blockSize` | Sum of chunk offsets |
| Implementation risk | Bitmap + mutex/atomic, slightly more code | Simpler scheduler |

Recommendation: **A** (this is what aria2/`curl` parallel schemes effectively do).
Auto-tune block size: `blockSize = clamp(size / (threads*4), 1 MiB, 32 MiB)` rounded to 1 MiB.

### Concurrency and connection reuse

- Default `--threads 4`, hard cap 32; `--threads 1` degenerates to the single-stream path.
- Each worker keeps issuing sequential range requests per block over a shared
  `http.Transport`; keep-alive reuses connections automatically.
- HTTP/2 streams share one TCP connection, which can cap throughput on high-BDP links.
  Go's default transport negotiates h2 for HTTPS; expose `--http1` to force HTTP/1.1
  (implemented by setting `TLSNextProto` to an empty map on the transport).
  Recommendation: keep the Go default, document `--http1` as a throughput escape hatch.

### Resume State

Sidecar JSON next to the output file: `<output>.part.mu-dl.json`; payload written to
`<output>.part`, renamed to `<output>` only after full verification.

```json
{
  "version": 1,
  "url": "https://example.com/foo.iso",
  "final_url": "https://cdn.example.com/foo.iso",
  "size": 1073741824,
  "etag": "\"abc123\"",
  "last_modified": "Wed, 21 Oct 2026 07:28:00 GMT",
  "block_size": 8388608,
  "total_blocks": 128,
  "done_blocks": [0, 1, 2, 5],
  "output": "/home/me/foo.iso",
  "started_at": "2026-01-01T00:00:00Z"
}
```

Rules:

- Resume is **on by default** when a state file exists (like `wget -c` / aria2);
  `--no-resume` discards the state and starts over.
- Validate before reuse: same `size`, and matching `ETag` (preferred) or
  `Last-Modified`. Mismatch → keep the old partial as `<output>.part.old`, start fresh
  with a visible warning. `--force-resume` overrides (only safe when the file is stable).
- Weak validators (`W/"..."`) are compared verbatim; if the server sends no validator,
  resume on size match only, with a warning.
- Blocks are marked done only after `WriteAt` returns; a crash between write and state
  flush merely re-downloads that block (idempotent).
- State is flushed at most once per second, plus on completion, error and signal.
- Preallocate the full file size up front (`Truncate`) so `WriteAt` offsets are valid
  and disk-space failures surface early.

### Stall detection and retries

- Per-block attempt: `--timeout` (default 30s) for connect/TLS/headers, plus an
  **idle-read timeout** (default 30s without a byte) so a dead connection fails fast
  instead of hanging the whole worker.
- On failure: retry the same block with exponential backoff (1s, 2s, 4s; `--retries 3`).
  Exhausted retries fail the whole download (partial file + state are kept for resume).
- `416 Range Not Satisfiable` at EOF is treated as "block already complete".
- A ranged request that unexpectedly returns `200` with the full body switches the run
  to single-stream mode (server ignored `Range`); this is detected on the first block.

### CLI Surface (draft)

```
mu network download <url> [flags]

  -o, --output PATH      Output file or directory (default: remote filename, CWD)
  -n, --threads N        Parallel connections (default 4, max 32)
      --no-resume        Ignore existing state/partial and start over
      --force-resume     Resume even if validators mismatch
  -f, --force            Overwrite an existing completed output file
      --block-size SIZE  Work unit size (default: auto, 1–32 MiB)
  -k, --insecure         Skip TLS verification
  -H, --header K:V       Extra request header (repeatable)
  -A, --auth TOKEN       Authorization: Bearer <token>
      --user USER:PASS   HTTP basic auth
      --proxy URL        HTTP(S) proxy for the transfer
      --timeout DUR      Connect/header timeout (default 30s)
      --idle-timeout DUR Stall timeout (default 30s)
      --retries N        Per-block retries (default 3)
      --limit-rate RATE  Global cap, e.g. 5M, 500k (default: unlimited, x/time/rate)
      --sha256 HEX       Expected checksum (fails the run on mismatch)
      --http1            Force HTTP/1.1 (disable HTTP/2 multiplexing)
      --no-progress      Disable the live display (line logs / silent when quiet)
      --quiet            No progress output, errors only
      --json             Print the final result as JSON (scripting)
```

Exit codes: `0` success, `1` generic failure (network/disk/checksum), `130` interrupted.
On interrupt the partial file and state stay on disk and a resume hint is printed.

### Progress Display

TTY, default (single aggregate line, 10 Hz):

```
foo.iso  45.2%  ████████████░░░░░░░░  452.1 MiB/1000.0 MiB  24.3 MiB/s  ETA 00:22  conn 6/8
```

- `conn 6/8` satisfies the "currently working threads" requirement (active workers over
  configured workers); a worker counts as active while it has an in-flight request.
- Speed is a sliding window (~3s) over the aggregate atomic byte counter, not a
  whole-run average, so the number reacts to real-time changes.
- `-v/--verbose` (or always when `--threads > 1`? — Q3) adds one line per connection:

```
  [1]  62.5 MiB  11.2 MiB/s  ██████░░░░
  [2]  58.1 MiB   9.8 MiB/s  █████░░░░░
  ...
```

- Non-TTY (piped/CI/log): one plain line per second, no ANSI, no carriage returns;
  the final summary is always printed. `MU_NO_COLOR` is honored.
- Renderer: hand-rolled ANSI using `aec` (already a dependency), matching the
  `internal/core/runner` display style. Alternative: add `schollz/progressbar/v3`
  or `cheggaaa/pb` — see Q4.
- Finish: clear the live region, print a summary
  (`path, size, elapsed, average speed, connections`).

## Edge Cases

| Case | Handling |
|---|---|
| `Content-Length` unknown / chunked | Single-stream mode, no resume (state file records `size: 0`) |
| `HEAD` returns 405/501 | Probe with `GET` + `Range: bytes=0-0`, inspect 206/`Content-Range` |
| Server returns 200 for a range request | Fall back to single-stream, restart from offset 0 |
| `Content-Disposition` filename with path separators | Sanitize with `filepath.Base`; never write outside `--output` |
| Output file already exists, complete, no state | Refuse unless `--force` |
| 0-byte remote file | Create the empty file, success |
| Disk full / write error | Abort, keep partial + state, exit non-zero |
| Ctrl-C (SIGINT/SIGTERM) | Cancel context, flush state, keep `.part`, print resume hint, exit 130 |
| Checksum mismatch | Keep the file, delete nothing, exit non-zero with expected/actual |
| Redirect chain | Follow (default client), apply ranges to the resolved URL, record `final_url` |
| Server limits per-IP connections (429/503) | Retry with backoff, and shrink worker count on repeated 429 (optional) |

## Testing

- `probe_test.go` — `httptest` servers: plain HEAD, HEAD 405 → GET+Range probe, no-range server, no `Content-Length`.
- `downloader_test.go` — full parallel download of a generated payload; `sha256` equality; `-race` clean.
- `resume_test.go` — server handler that closes the connection after N bytes; restart the download; assert only missing blocks are fetched (request-range assertions) and the final file matches.
- `state_test.go` — corrupt/truncated JSON tolerated; validator mismatch handling; atomic save.
- `single_stream_test.go` — range-ignoring server fallback.
- Manual: a large real asset (e.g. an Ubuntu ISO or a big GitHub release asset), interrupt + resume, compare `sha256`.

## Implementation Steps

| Step | Content |
|------|---------|
| 1 | `internal/core/downloader`: `Probe` + single-stream path + tests |
| 2 | Block plan/bitmap scheduler, workers, `WriteAt`, preallocation, `-race` tests |
| 3 | State file load/save/validate + resume + `--no-resume`/`--force-resume` |
| 4 | Retry/backoff, idle-stall timeout, 416 handling, graceful Ctrl-C |
| 5 | `stats.go` + `internal/network/download.go` CLI flags wiring |
| 6 | `internal/network/progress.go`: aggregate line, per-connection lines, non-TTY fallback |
| 7 | `--sha256`, `--limit-rate`, `--json`, `--http1`, proxy/auth/headers |
| 8 | Register subcommand; update `docs/network.md`, `README.md`, `CODEBASE.md` |
| 9 | `gofmt` / `go vet` / `go test -race ./...`; cross-platform build check (`make all`) |

## Confirmed Decisions

| # | Question | Decision |
|---|---|---|
| Q1 | Segmentation | **Block queue + bitmap** (default block 8 MiB, auto-tuned 1–32 MiB, `WriteAt` into a preallocated file) |
| Q2 | Resume default | **Automatic** when `<output>.part.mu-dl.json` exists; `--no-resume` starts over, `--force-resume` skips validator checks |
| Q3 | Progress layout | **Single aggregate line by default**, `-v` adds one line per connection |
| Q4 | Progress rendering | **Hand-rolled ANSI** on top of `aec`; no new progress-bar dependency |
| Q5 | Web UI | **CLI only in v1** (no `serve` tab / SSE endpoint); revisit later |
| Q6 | v1 extras | **All included**: `--sha256`, `--limit-rate` (`x/time/rate`), `--proxy`, `--http1` |

### Deferred / out of scope for v1

- Re-pointing `mu network http -o` at the downloader (keep it single-connection for now).
- Web UI download tab with SSE progress.
- Archive auto-extraction, FTP/SFTP/BitTorrent, HLS/DASH assembly.
