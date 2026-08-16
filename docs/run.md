# run — Execute commands with colored output

```bash
mu run --command "echo hello" --command "ls -la"
mu run --command "greet::echo hello"   # optional name prefix: <name>::<command>
mu run --command "!sudo whoami"        # prefix ! = interactive (wire terminal stdin/output)
```

Each command runs sequentially through `bash -c`. When stdout is a terminal,
non-interactive commands run on a **pseudo-terminal** so stdio programs
line-buffer and stream output promptly; the output is rendered through a VT100
emulator (handling ANSI, wrapping and CR like a real terminal) in a **live
per-step region** (6 rows by default), updated in place as the command runs.
The step header shows an animated spinner plus the elapsed time while running,
and on completion the region is cleared and the header collapses to a green
`Executing [name]... ✓ 0.8s` line (red `✗` on failure, followed by the failed
step's recent output and an `Error:` line) and the runner stops.

Behavior can be tuned:

- **Live region height** — set `MU_RUN_LOG_LINES` (e.g. `10`) to change the
  per-step display rows, mirroring `BUILDKIT_TTY_LOG_LINES`.
- **Interactive commands** (`!` prefix) — suspend the live display and connect
  stdin/stdout/stderr directly to your terminal, so prompts (e.g. `sudo`
  password, `apt` y/n) work normally.
- **Piped output** — when stdout is not a TTY (piped or redirected), the display
  machinery is skipped and output is printed as plain `Executing [name]...` /
  `name ✓ (0.8s)` lines.
- **Interrupt** — `Ctrl-C` forwards the signal to the currently running command,
  stops after it, and exits with code `130` (press `Ctrl-C` twice to force-quit).
- **Color** — name lines are blue (cyan on Windows), `✓` is green, errors are
  red; set `NO_COLOR` to disable.

## Task orchestration with recipe files

`mu run --file <recipe.yaml>` runs a set of named, ordered tasks with
dependencies, variables, timeouts, retries, and failure policies, reusing the
same live display (each task is one step). `--command` and `--file` are
mutually exclusive.

```yaml
# tasks.yaml
name: build-deploy
vars:                        # {{.key}} templates, overridable with --var key=value
  version: "1.2.3"
  registry: "ghcr.io/me/app"
env:                         # applied to every task
  CI: "true"
tasks:
  clean:
    command: rm -rf dist
    timeout: 30s             # per-task timeout, e.g. "30s" or "2m"
  build:
    depends: [clean]         # runs after clean completes
    commands:                # multiple commands run sequentially
      - go vet ./...
      - go build -o dist/app .
    env:
      VERSION: "{{.version}}"
    retry: 2                 # retry this many extra times on failure
  test:
    depends: [build]
    command: go test ./...
    continue_on_error: true  # keep going even if this task fails
  deploy:
    depends: [build]
    workdir: ./deploy        # default working directory is the current directory
    command: "kubectl set image deploy/app app={{.registry}}:{{.version}}"
```

```bash
mu run --file tasks.yaml                    # run all tasks in dependency order
mu run --file tasks.yaml --task build,test  # run only these + their dependencies
mu run --file tasks.yaml --var version=2.0  # override a variable
mu run --file tasks.yaml --keep-going       # continue after failures
mu run --file tasks.yaml --dry-run          # print the ordered task list, don't run
mu run --schema                             # print the recipe JSON Schema
```

Recipe semantics:

- **Dependency order** — tasks are scheduled topologically; a task runs after
  all of its `depends`. Cycles are rejected at load time.
- **Variables** — `{{.key}}` is templated with `vars` merged with `--var`
  overrides; a reference to an undefined variable is an error.
- **Timeout** — a task exceeding `timeout` is killed and reported as
  `timed out`.
- **Retry** — a failed task is retried up to `retry` more times (each attempt
  re-runs the full task).
- **Failure policy** — by default the run stops at the first failure; a task
  with `continue_on_error: true`, or `--keep-going`, lets later tasks run. The
  final summary lists each task with `✓`/`✗` and its duration, and any failure
  yields a non-zero exit code.
- **Interactive commands are not supported** in recipes — a `!`-prefixed
  command is rejected at load time, and commands that try to read stdin fail
  fast at runtime.
- **Editor validation** — the recipe JSON Schema lives at
  `schema/recipe-schema.json` (also embedded and printed by
  `mu run --schema`). Add a header to a recipe file to enable autocomplete and
  inline validation in editors backed by the YAML Language Server:
  `# yaml-language-server: $schema=https://raw.githubusercontent.com/yusiwen/myUtilities/main/docs/schema/recipe-schema.json`
  (pin the URL to a release tag for stability; breaking format changes bump the
  schema's `x-recipe-version`). Editor validation is optional and independent
  of the runtime checks.
