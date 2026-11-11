# fleet — Remote batch execution and deployment

Run commands or recipes across multiple hosts via a **dispatcher + agent**
model (like a small ansible). A controller (e.g. your laptop) submits a job to
a dispatcher (`mu fleet serve`); agents (`mu fleet agent`) running on each
target host poll for work, execute it with the `run` engine, and stream output
back. Jobs survive the controller going offline, and temporarily offline hosts
pick up pending jobs when they return.

```bash
# Start the dispatcher (controller endpoint). Requires a token — see
# "Auth and security" — and binds loopback by default.
mu fleet serve

# Let agents on other hosts reach it (exposes the port; the token still applies)
mu fleet serve --host 0.0.0.0

# On each target host: run the agent, pointing at the dispatcher
mu fleet agent --server http://laptop:8890 --groups prod

# Quick command on one or more hosts
mu fleet run --hosts h1,h2 --command "systemctl restart nginx"

# Recipe (task orchestration) with variables
mu fleet run --hosts prod-web-1,prod-web-2 --file deploy.yaml --var version=2.0 --watch

# Deploy with an uploaded archive (auto-extracted on each host)
mu fleet run --hosts h1,h2 --file deploy.yaml --files dist/app.tar.gz --watch

# Status and operations
mu fleet hosts             # online agents
mu fleet status <job-id>   # per-host status and output
mu fleet jobs              # recent jobs
```

## Workflows

### Quick command — no files

```bash
mu fleet run --hosts h1,h2 --command "systemctl restart nginx && systemctl status nginx"
```

1. The controller reads `~/.config/mu/fleet-config.json` for the dispatcher URL and token.
2. The controller submits the job in one **multipart** request: `POST /api/fleet/jobs`,
   with JSON metadata (the `command` text, `targets: ["h1","h2"]`, no files).
3. The dispatcher creates the job and enqueues one `run` (state `pending`) per target
   host; it returns the job id.
4. The agents on h1/h2 each poll `POST /api/fleet/agents/<name>/poll` and claim their
   own `pending` run.
5. Each agent executes the command via `internal/core/runner` (headless plain mode),
   streaming output back in chunks: `POST /api/fleet/agents/<name>/runs/<id>/output`.
6. On completion the agent reports the result: `POST .../complete` (success/failure +
   exit code + elapsed time).
7. With `--watch` the controller polls `GET /api/fleet/jobs/<id>` every ~1s to refresh
   each host's output live; without `--watch` it returns the current state once.
8. A per-host summary (✓/✗ + elapsed) is printed; any failure yields a non-zero exit code.

### Deploy with an uploaded archive — file transfer + auto-extract

```bash
./build.sh                                    # build dist/app.tar.gz locally
mu fleet run --hosts h1,h2 --file deploy.yaml \
  --files dist/app.tar.gz --var version=1.2
```

1. The controller uploads the local `dist/app.tar.gz` as a multipart file segment
   with the job; the dispatcher computes and records the sha256, storing it under
   `~/.cache/mu/fleet/jobs/<id>/files/`.
2. After claiming a run, the agent downloads the task files:
   `GET /api/fleet/agents/<name>/runs/<id>/files/app.tar.gz`, verifying the sha256.
3. The agent creates a job work directory, places the file in it, and **auto-extracts
   `.tar.gz`** (`.zip` works the same way).
4. The agent runs `deploy.yaml` with that work directory as the recipe workdir
   (format: [Task YAML: the recipe schema](#task-yaml-the-recipe-schema)), so
   relative paths naturally resolve inside the task directory:

   ```yaml
   tasks:
     install:
       command: cp app /usr/local/bin/app
     restart:
       depends: [install]
       command: systemctl restart app
   ```

5. Output streaming and result reporting match the quick-command flow; the task
   directory is discarded after use and never pollutes the host.
6. A summary of each host's deployment result is printed.

### Batch with multiple variables and partial failure

```bash
mu fleet run --hosts prod-web-1,prod-web-2,prod-db-1 --file rollout.yaml \
  --var version=2.0 --var registry=ghcr.io/me/app --watch
```

1. All three hosts claim runs in parallel (no blocking).
2. Template variables like `{{.version}}` and `{{.registry}}` are resolved on **each
   agent** (reusing `core/runner`'s recipe templating).
3. If one host fails, **that host** stops per the recipe's failure policy (default
   stop; a task with `continue_on_error: true` excepted) while the other hosts
   continue. `mu fleet run` has no `--keep-going` flag — that switch belongs to
   `mu run`.
4. The dispatcher records each run's state; with `--watch` the summary lists per-host
   results and highlights the partial failure.

### Offline host catch-up — the core value of the poll model

1. If `prod-db-1` is offline when the job is submitted, the dispatcher still keeps its
   `pending` run (nothing is lost).
2. When `prod-db-1` comes back online, its agent polls, claims the run, downloads files,
   executes, and reports.
3. `mu fleet status <job-id>` later still shows that host's full result and historical
   output (persisted in BoltDB).

### Status and operations

```bash
mu fleet hosts            # online agents (offline after heartbeat timeout)
mu fleet status <job-id>  # per-host status and output (including history)
mu fleet jobs             # recent jobs
```

## Task YAML: the recipe schema

`mu fleet run --file <recipe.yaml>` defines no YAML dialect of its own: the file is
shipped as text and parsed on each agent by the same engine as `mu run --file`, so a
fleet job *is* a recipe (see [run.md](run.md)). Fleet adds only job-level context:
`targets` (from `--hosts`), `vars` (repeated `--var key=value`), and uploaded files
(`--files`).

```yaml
name: deploy-app                  # optional, informational
description: Roll out v2          # optional, informational
vars:                             # optional; referenced as {{.key}}
  version: "2.0"
  registry: "ghcr.io/me/app"
env:                              # optional; applied to every task
  DEPLOY_ENV: prod
tasks:                            # required, at least one task
  install:
    command: cp app /usr/local/bin/app
  restart:
    depends: [install]            # runs after `install`
    command: systemctl restart app
    timeout: "30s"
    retry: 2                      # up to two extra attempts
    continue_on_error: false
```

Top-level fields:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `name` | string | no | Recipe name. |
| `description` | string | no | Free-form description. |
| `vars` | map of string | no | Template variables; `--var` overrides a key. |
| `env` | map of string | no | Environment variables for every task. |
| `tasks` | map of task | **yes** | Named tasks, at least one. |

Task fields:

| Field | Type | Meaning |
|---|---|---|
| `command` | string | One `bash -c` command line. |
| `commands` | list of string | Several command lines, run in order as one step. |
| `depends` | list of string | Task names that must finish first; cycles are rejected. |
| `env` | map of string | Extra environment variables for this task, merged over the recipe-level `env`. |
| `workdir` | string | Working directory; a relative path resolves against the job's staging directory. |
| `timeout` | string | Go duration such as `"30s"` or `"2m"`; the task is killed when it expires. |
| `retry` | integer ≥ 0 | Extra attempts after a failure. |
| `continue_on_error` | boolean | Keep running later tasks when this one fails. |
| `description` | string | Free-form description. |

Exactly one of `command` and `commands` must be present, and unknown keys anywhere in
the file are rejected.

The loader checks the following before anything executes on the host:

- unknown keys at any level (`field bogus not found in type runner.RecipeTask`);
- `command` together with `commands`, or neither of them;
- a `depends` entry naming no task, or a dependency cycle;
- a `timeout` that is not a Go duration, or a negative `retry`;
- an interactive command (a `!` prefix) — agents run headless, with no TTY.

Variables: `vars` is merged with `--var key=value` (the flag wins, and a key need not be
declared first). `{{.key}}` is expanded in `command`, `commands`, `workdir`, and `env`
values, at both recipe and task level; an undefined variable is an error
(`missingkey=error`), never an empty string. A literal `{{` therefore has to be written
as `{{"{{"}}`, exactly as in a command.

Validation happens on the agent, not at submit time: the controller only reads the file,
and the dispatcher only requires `recipe` (or `command`) to be non-empty. A malformed
recipe therefore fails **per host** — the parse error is recorded on that host's run and
appears in `mu fleet status <job-id>` and in `--watch` output, while unaffected hosts
still run.

Editor support: `mu run --schema` prints the JSON Schema
(`docs/schema/recipe-schema.json`, embedded in the binary). Add this header to a recipe
to get completion and inline errors from editors backed by the YAML Language Server:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/yusiwen/myUtilities/main/docs/schema/recipe-schema.json
```

Breaking format changes bump the schema's `x-recipe-version`.

## Architecture

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

## Dispatcher API (`X-Auth-Token` auth)

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

Job state derivation: all runs `succeeded` → `succeeded`; any `failed` → `failed`;
otherwise `pending`/`running`.

## File transfer and extraction

- The controller uploads all `--files` in one multipart request; the dispatcher records
  sha256 and stores them on disk.
- The agent verifies the sha256 after download and places the files in the job work directory.
- `.tar.gz` / `.zip` archives are auto-extracted (like ansible `unarchive`); other files
  are placed as-is.
- Recipes use the job work directory as their default workdir; the directory is
  discarded after the run.

## Auth and security

- **A token is required.** The dispatcher refuses to start when neither `token` in
  `fleet-config.json` nor `--token` is set. The library side fails closed too: a
  dispatcher built without a token rejects every request with `401` instead of
  serving anonymously.
- The token is sent as `X-Auth-Token` on every request (same pattern as WOL);
  `fleet-config.json` is created with `0600` permissions.
- **The dispatcher binds `127.0.0.1` by default.** Pass `--host 0.0.0.0` (or a
  specific address) to let agents on other hosts reach it — the token is then the
  only thing between the network and job execution.
- `--allow-anonymous` serves without authentication for throwaway local testing.
  It cannot be combined with a configured token (so a token is never silently
  ignored), it logs a warning at start-up, and it should stay on loopback.
- **Risk note:** an agent executes arbitrary commands submitted by the controller —
  this is an RCE surface. Keep the dispatcher on a trusted network and treat the
  token as a root credential. Future hardening could use mTLS or per-agent
  credentials.

## Config

`~/.config/mu/fleet-config.json` (server, token, hostname, groups, poll_interval,
port, db_path, data_dir); `--config` overrides the path. Design details:
[plan/fleet-module-plan.md](plan/fleet-module-plan.md).