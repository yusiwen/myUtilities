# mu fleet module plan (remote batch execution and deployment)

## 1. Complete workflows for different scenarios

### Scenario A: Quick ad-hoc operation (single command, no files)

```bash
mu fleet run --hosts h1,h2 --command "systemctl restart nginx && systemctl status nginx"
```

Full flow:

1. The controller (MacBook) reads `~/.config/mu/fleet-config.json` for the dispatcher URL and token.
2. The controller submits the job in one **multipart** request: `POST /api/fleet/jobs`, carrying JSON metadata (the `command` text, `targets: ["h1","h2"]`, no files).
3. The dispatcher creates the job and enqueues one `run` (state `pending`) per target host; it returns the job id.
4. The agents on h1/h2 (`mu fleet agent`) each poll `POST /api/fleet/agents/<name>/poll` and claim their own `pending` run.
5. The agent executes the command via `internal/core/runner` (headless plain mode), streaming output back in chunks: `POST /api/fleet/agents/<name>/runs/<id>/output`.
6. On completion the agent reports the result: `POST .../complete` (success/failure + exit code + elapsed time).
7. With `--watch` the controller polls `GET /api/fleet/jobs/<id>` every ~1s to refresh each host's output live; without `--watch` it returns the current state once.
8. A per-host summary is printed (✓/✗ + elapsed time); any failure yields a non-zero exit code.

### Scenario B: Deployment with an archive (file transfer + auto-extract)

```bash
./build.sh                                    # local build produces dist/app.tar.gz
mu fleet run --hosts h1,h2 --file deploy.yaml \
  --files dist/app.tar.gz --var version=1.2
```

Full flow:

1. The controller uploads the local `dist/app.tar.gz` as a multipart file segment with the job; the dispatcher computes and records the sha256, storing it under `~/.cache/mu/fleet/jobs/<id>/files/`.
2. After claiming a run, the agent downloads the task files: `GET /api/fleet/agents/<name>/runs/<id>/files/app.tar.gz`, verifying the sha256.
3. The agent creates a job work directory, places the file in it, and **auto-extracts `.tar.gz`** (`.zip` works the same way).
4. The agent runs `deploy.yaml` with that work directory as the recipe workdir, so relative paths naturally resolve inside the task directory:
   ```yaml
   tasks:
     install:
       command: cp app /usr/local/bin/app
     restart:
       depends: [install]
       command: systemctl restart app
   ```
5. Output chunking and result reporting match scenario A; the task directory is discarded after use and never pollutes the host.
6. A summary of each host's deployment result is printed.

### Scenario C: Batch + multiple variables + partial failure

```bash
mu fleet run --hosts prod-web-1,prod-web-2,prod-db-1 --file rollout.yaml \
  --var version=2.0 --var registry=ghcr.io/me/app --watch
```

Full flow:

1. All three hosts claim runs in parallel (no blocking).
2. Template variables like `{{.version}}` and `{{.registry}}` are resolved on **each agent** (reusing `core/runner`'s recipe templating).
3. If one host's task fails, **that host** stops per the recipe's failure policy (default stop; `continue_on_error` / `--keep-going` excepted), while the other hosts continue.
4. The dispatcher records each run's state; with `--watch` the summary lists per-host results and highlights the partial failure.

### Scenario D: Offline host catch-up (the core value of the poll model)

1. If `prod-db-1` is offline when the job is submitted, the dispatcher still keeps its `pending` run (nothing is lost).
2. When `prod-db-1` comes back online, the agent polls, claims the run, downloads files, executes, and reports.
3. `mu fleet status <job-id>` later still shows that host's full result and historical output (persisted in BoltDB).

### Scenario E: Status and operations

```bash
mu fleet hosts            # online agent list (offline after heartbeat timeout)
mu fleet status <job-id>  # per-host task status and output (including history)
mu fleet jobs             # recent job list
```

## 2. Background and motivation

`mu run --file <recipe.yaml>` turned single-machine task orchestration into a "local mini-CI", but deployments usually need to **execute on multiple hosts in batch**. This module provides an ansible-like capability: one machine (e.g. a MacBook) initiates remote batch execution and deployment.

Design trade-offs:

- **Model A (dispatcher + agent poll)** instead of ansible-style push/direct connection:
  - agents need no inbound ports (suitable for homelab / NAT);
  - dispatched tasks **do not depend on the initiator staying online** (disconnect after submitting; agents keep running; check results when back);
  - temporarily offline hosts catch up when they return;
  - single-point authentication.
- Heavy reuse of existing assets: `core/runner` (recipe execution engine), `core/wol/agent.go` (registration + backoff retry pattern), `core/store` (BoltDB), module config conventions.

## 3. Overall architecture

```
MacBook (controller) ──HTTP──▶ mu fleet serve (dispatcher)
                                    │  └─ BoltDB: agents / jobs / runs / run_output
                                    │  └─ files: ~/.cache/mu/fleet/jobs/<id>/files/
                    ┌───────────────┼───────────────┐
                 h1 agent        h2 agent        h3 agent
              (mu fleet agent)  (poll claim + local execute)
```

Data flow:

- Controller → dispatcher: submit job (multipart, may include files), query status/output.
- Dispatcher → agent: poll returns a pending run (recipe/command, vars, file manifest).
- Agent → dispatcher: register, output chunks, completion result, heartbeat (poll doubles as heartbeat).
- Agent → dispatcher file download: pull task files after claiming a run.

## 4. Directory structure

```
internal/core/fleet/
  ├── types.go        Agent / Job / JobRun / state constants
  ├── store.go        BoltDB persistence (agents / jobs / runs / run_output)
  ├── dispatcher.go   dispatcher HTTP handlers + RegisterHandlers
  ├── auth.go         X-Auth-Token middleware
  ├── agent.go        agent loop (register → poll → execute → report)
  ├── client.go       HTTP client for controller / agent
  ├── transfer.go     file upload/download + archive extraction (.tar.gz / .zip)
  └── *_test.go       unit + integration tests

internal/fleet/       CLI wrapper (parse only + call core)
  ├── options.go      subcommand and option definitions
  ├── command.go      Run() implementations
  └── config.go       fleet-config.json load/save

cmd/mu/myutilities.go  register the Fleet top-level command
```

All core logic lives in `internal/core/fleet`; `internal/fleet` is only a CLI wrapper (following project conventions).

## 5. CLI design

```
mu fleet serve [--port 8890]
mu fleet agent [--server URL] [--hostname NAME] [--groups prod] [--poll-interval 5s]
mu fleet run --hosts h1,h2 [--file x.yaml | --command "cmd"] [--var k=v] [--files path...] [--watch]
mu fleet hosts
mu fleet status <job-id> [--watch]
mu fleet jobs [--limit 20]
```

Rules:

- `run`'s `--file` and `--command` are mutually exclusive (exactly one required).
- `--hosts` is required, comma-separated or repeated.
- `--files` is repeatable; `--var` is repeatable.
- Config: `~/.config/mu/fleet-config.json`, fields: `server`, `token`, `hostname`, `groups`, `poll_interval`, `port`, `db_path`, `data_dir`; `--config-dir` overrides the base directory; contains secrets → file permission `0600`.

## 6. Data model and storage

```
Agent   { hostname, groups, lastSeen }
Job     { id, recipe text | command, vars, targets, files[] {name,size,sha256}, createdAt }
JobRun  { jobID+hostname, state(pending/running/succeeded/failed), startedAt, finishedAt, error }
```

BoltDB buckets:

| Bucket | Key → Value |
|---|---|
| `agents` | hostname → serialized Agent |
| `jobs` | job id → serialized Job |
| `runs` | `<jobID>/<hostname>` → JobRun metadata |
| `run_output` | `<jobID>/<hostname>` → accumulated output |

- **Output persistence**: agents report in chunks (roughly every 100ms or every N lines); the dispatcher appends each batch to `run_output` to avoid per-line writes. Each run's output is truncated at 1MB by default (configurable); past the limit the tail is kept and marked `[truncated]` to prevent unbounded growth.
- After a dispatcher restart: agents/jobs/runs/run_output are all restored — tasks are not lost and historical output is queryable.
- Job files are stored under `~/.cache/mu/fleet/jobs/<id>/files/`, isolated per task.
- Offline agents' pending runs **stay waiting** (for catch-up); an agent's Online state is derived dynamically from lastSeen.

## 7. Dispatcher API (`X-Auth-Token` auth)

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/fleet/jobs` | Controller submits a job (multipart: `job` JSON field + `file` segments) |
| GET | `/api/fleet/jobs` | Recent job list |
| GET | `/api/fleet/jobs/{id}` | Job status and per-host output |
| POST | `/api/fleet/register` | Agent registration (hostname/groups) |
| POST | `/api/fleet/agents/{name}/poll` | Agent claims a pending run + heartbeat |
| POST | `/api/fleet/agents/{name}/runs/{id}/output` | Reports an output chunk |
| POST | `/api/fleet/agents/{name}/runs/{id}/complete` | Reports completion result |
| GET | `/api/fleet/agents/{name}/runs/{id}/files/{name}` | Agent downloads a task file |
| GET | `/api/fleet/agents` | Online agent list |

Job state derivation: all runs `succeeded` → `succeeded`; any `failed` → `failed`; otherwise `pending`/`running`.

## 8. Agent lifecycle

1. **Register**: `POST /api/fleet/register` with hostname/groups; retry with backoff on failure (reusing `core/wol`'s `agentMaxRetries` pattern).
2. **Loop**:
   - `poll` (doubles as heartbeat, updates lastSeen on the dispatcher side): if a `pending` run exists, claim it; otherwise sleep `poll_interval`.
   - After claiming: download task files → verify sha256 → extract archives → execute in the task work directory (recipe via `RunRecipe`, command via `Run`, both in core/runner plain mode) → POST output chunks while running → POST complete when finished.
3. Reconnection and heartbeat timeout: the dispatcher marks agents offline based on lastSeen timeout (default 3×poll_interval).

## 9. File transfer and archive extraction

- The controller uploads all `--files` in one multipart request; the dispatcher records sha256 and stores them on disk.
- The agent verifies the sha256 after download and places the files in the task work directory.
- `.tar.gz` / `.zip` are auto-extracted (like ansible `unarchive`); other files are placed as-is.
- Recipes use the task work directory as their default workdir; the directory is discarded after use.

## 10. Auth and security

- Shared token: every request carries `X-Auth-Token` (same pattern as WOL); `fleet-config.json` permission `0600`.
- **Risk note**: an agent executes arbitrary commands submitted by the controller = RCE surface; only suitable for a trusted LAN. Future hardening could use mTLS / per-agent credentials.

## 11. Small core/runner changes

- Plain output currently writes directly to `os.Stdout`. Add an injectable `io.Writer` to `CommandRunner` (default `os.Stdout`) so the agent can tee output to a chunked report channel; the TTY display path is unaffected.
- Add `ParseRecipe([]byte)` to recipe parsing so the agent can parse dispatched recipe text directly (no temp files).
- `RunRecipe` already returns `[]TaskResult` for the agent to aggregate.

## 12. Test plan

- Unit: `store` (CRUD / output append / truncation / Online derivation), dispatcher handlers (`httptest`, including auth and file upload/download), `transfer` (archive extraction), agent loop (fake dispatcher), runner OutputWriter.
- Integration: spin up an in-process dispatcher + agent + controller client, run a small recipe and a task with `--files`, assert per-host results, output persisted to DB, files landed and extracted.
- Keep the existing baseline of 264 tests green.

## 13. Documentation updates

- `README.md` gains an accurate `### fleet` section (command usage + workflow examples).
- `AGENTS.md` gains core/fleet and command/fleet entries.
- `CODEBASE.md` directory tree updated.

## 14. Phases

- **Phase 1 (this plan)**: everything above — serve/agent/run/hosts/status/jobs + file transfer and auto-extract + token auth + BoltDB (including output persistence) + polled output (`--watch`).
- **Phase 2**: SSE/WebSocket real-time output streaming, inventory group aliases (`--hosts groupname`), gateway task/host web pages, mTLS.