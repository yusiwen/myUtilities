# fleet — Remote batch execution and deployment

Run commands or recipes across multiple hosts via a **dispatcher + agent**
model (like a small ansible). A controller (e.g. your laptop) submits a job to
a dispatcher (`mu fleet serve`); agents (`mu fleet agent`) running on each
target host poll for work, execute it with the `run` engine, and stream output
back. Jobs survive the controller going offline, and temporarily offline hosts
pick up pending jobs when they return.

```bash
# Start the dispatcher (controller endpoint)
mu fleet serve

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

Semantics:

- **Model** — agents poll the dispatcher (`X-Auth-Token` shared token), so no
  inbound ports are needed on hosts; dispatched jobs keep running even if the
  controller disconnects.
- **Execution** — `--file` (recipe) and `--command` are mutually exclusive;
  recipes support `depends`, `vars`, `workdir`, `timeout`, `retry`,
  `continue_on_error` exactly like `mu run --file`.
- **Files** — `--files` uploads artifacts with the job; agents download them,
  verify SHA-256, and auto-extract `.tar.gz`/`.zip` into the job's work
  directory.
- **Output** — streamed back in chunks and persisted in the dispatcher's
  BoltDB; `--watch` polls until completion and prints a per-host summary.
- **Config** — `~/.config/mu/fleet-config.json` (server, token, hostname,
  groups, poll_interval, port, db_path, data_dir); `--config` overrides the
  path. Design details: `plan/fleet-module-plan.md`.
