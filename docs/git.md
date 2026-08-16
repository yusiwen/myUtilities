# git — Git utilities with AI

Subcommands: `commit`, `review`, `ignore`.

## git commit — AI-generated conventional commit messages

Generates a conventional commit message from staged changes using an LLM.

```bash
# Generate and confirm
mu git commit

# Skip confirmation (auto-commit)
mu git commit --yes

# Chinese commit message
mu git commit --lang cn

# Debug: print full prompt, API request/response, timing
mu git commit --verbose

# Dry run: print message without committing
mu git commit --dry-run
```

## git review — AI-powered code review (Agent mode)

Analyzes local changes using a multi-turn LLM agent. The agent can read files, search
code, and inspect diffs before producing a structured markdown review.

```bash
# Review unstaged changes
mu git review

# Review staged changes
mu git review --staged

# Branch comparison
mu git review --base origin/main

# Compare two commits (see warning below)
mu git review --base <hashA> --target <hashB>

# Chinese output
mu git review --lang cn

# Extra context for the reviewer
mu git review --context "focus on error handling"

# List saved reviews (current project only)
mu git review --list

# List all saved reviews
mu git review --list --list-all

# Limit tool call rounds
mu git review --max-turns 10

# Disable SCIP semantic tools (e.g. on very large repos)
mu git review --no-scip

# Force regeneration of the SCIP index
mu git review --refresh-scip
```

> **`--target` consistency limitation:** `--base`/`--target` review committed
> ranges via `git diff <base>..<target>`. The agent's context tools (`read_file`,
> `read_function`, `search_code`, and the SCIP index) always operate on the
> **working tree**, which reflects the current `HEAD`. Reviewing a range whose
> `--target` is *not* the current `HEAD` would pair the target's diff with the
> HEAD's file/index content, producing potentially wrong results — `mu git
> review` therefore rejects it. To review an older target, first check it out
> (`git checkout <hash>`) then run `mu git review --base <hash>` (target
> defaults to `HEAD`). A dirty working tree during a committed-range review
> prints a warning.

> **Empty-diff hints:** like `git diff`, `git review` ignores untracked and
> staged files by default. When there is nothing to review it explains why and
> how to proceed: untracked files (`git add -N <file>` to include them), staged
> files (`mu git review --staged`, or `git reset` to unstage).

The review is rendered with syntax highlighting via `glamour`, paginated through
`less -R` (or `$PAGER`), and saved to:

```
~/.cache/mu/git_reviews/<project>_<branch>_<timestamp>.md
```

Saved files include YAML front matter with review metadata (commit, branch, diff stat,
strategy, timestamp, etc.).

## SCIP semantic code intelligence

`git review` uses **SCIP** (Sourcegraph's language-agnostic symbol index, the successor
to LSIF) to give the review agent precise semantic understanding of the codebase.
The agent gains four semantic tools:

| Tool | Purpose |
|------|---------|
| `find_references` | All usages/call sites of a symbol across the repo — assess the impact of changing or deleting a function |
| `find_definition` | Jump to the definition of a symbol referenced in the diff |
| `symbol_info` | Hover-style signature, kind, and doc comment |
| `read_function` | Reads the exact enclosing function body (upgraded from a fixed ±30-line window) |

Indexers are installed **on demand, treesitter-nvim style**: on the first review the
language is auto-detected from the repo (e.g. `go.mod`, `pom.xml`, `Cargo.toml`), the matching
indexer binary is downloaded from a GitHub release into `~/.cache/mu/scip/tools/`, and the index
is built and cached per commit in `~/.cache/mu/scip/index/`. Dirty working trees use a
`working` index (rebuilt when source files changed). A stale index is rebuilt with an
explanatory line; building shows a spinner.

Indexer output is captured to a temp file (streamed live with `--verbose`); on a failed
build the error lines are extracted and the full log is kept, e.g.:
`Full indexer log kept at: /tmp/scip-index-XXX`. Go and Rust failures degrade gracefully to
text tools, while a **Java build failure aborts the review** (fail fast), since `scip-java`
runs a real Maven/Gradle build.

SCIP management commands:

```bash
# Install the indexer for a language (auto-downloaded)
mu scip install go
mu scip install java
mu scip install rust
mu scip install go --release v0.3.0   # install a specific release tag

# List available / installed indexers (configured / pinned / installed versions)
mu scip list

# Build the index for the current repo
mu scip index

# Update indexer(s) to the latest release (persists the version in config)
mu scip update go              # update a single language
mu scip update                 # update all enabled indexers
mu scip update --dry-run       # show old → new without downloading
mu scip update --no-pin        # download only, don't touch git-config.json
mu scip update --keep-old      # keep previous versions instead of removing them

# Remove all cached indexers and indexes
mu scip purge
```

**Indexer version control:** each language's indexer release tag is pinned in
code as a conservative default (e.g. scip-go `v0.2.7`, scip-java `v0.13.1`,
rust-analyzer `2026-08-03`).
`mu scip update` upgrades to the latest release and records the new tag in
`git-config.json` under `review.scip.versions`, so upgrades are explicit and
persistent. Overrides can also be set manually:

```bash
mu set git review --scip-version go=v0.3.0      # pin a specific release
mu set git review --scip-version-rm go          # remove the override
```

**Java projects:** `scip-java` indexes by actually running the Maven/Gradle build
(`clean verify -DskipTests` / `clean compileTestJava...`), so it needs:

- JDK 17+ (`java` on `PATH`)
- a Maven or Gradle project whose build can compile cleanly
- network access to resolve dependencies on the first run (takes minutes)

`git review` builds the Java index automatically when missing or stale (fail-fast on a
failed build — see the retained-log hint above); use `--refresh-scip` to force a rebuild.

**Rust projects:** `git review` uses `rust-analyzer scip` to index Cargo workspaces, so it needs:

- `cargo` and `rustc` on `PATH`
- a Cargo workspace that resolves cleanly (indexing loads the whole workspace)

The rust-analyzer release asset is a bare gzipped binary (not a tar archive) and has no
companion SHA256 checksum, so it is downloaded without checksum verification. `rust-analyzer`
builds its SCIP index via load-bearing inference; macro/generic-heavy code may resolve less
precisely than with `scip-go`.

Configuration lives in `git-config.json` under the `review.scip` key:

```json
{
  "review": {
    "provider": "default",
    "lang": "en",
    "scip": {
      "enabled": true,
      "auto_install": true,
      "cache_dir": "",
      "versions": { "go": "v0.3.0" }
    }
  }
}
```

`cache_dir` defaults to `~/.cache/mu/scip`. Disabling `enabled` or `auto_install` makes
reviews silently fall back to text tools.

## git ignore — Download .gitignore templates

Downloads .gitignore templates from the [github/gitignore](https://github.com/github/gitignore) repository.

```bash
# List available templates
mu git ignore list

# Auto-detect language and download template
mu git ignore

# Download a specific template
mu git ignore Go

# Merge with existing .gitignore
mu git ignore Python --merge
```


## Configuration

LLM settings shared by `git commit` and `git review` are stored in `~/.config/mu/git-config.json`:

```json
{
  "providers": [
    {
      "name": "default",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "sk-xxx",
      "model": "deepseek-v4-flash"
    }
  ],
  "commit": {
    "provider": "default",
    "lang": "en"
  },
  "review": {
    "provider": "default",
    "lang": "cn",
    "reviews_dir": "~/.cache/mu/git_reviews"
  }
}
```

Configure via `mu set git`:

```bash
# Add a provider
mu set git provider add --name default --base-url <url> --api-key <key> --model <model>

# Remove a provider
mu set git provider rm --name <name>

# List providers
mu set git provider list

# Configure module defaults
mu set git commit --provider default --lang en
mu set git review --provider default --lang cn --reviews-dir ~/reviews
```
