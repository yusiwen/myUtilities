# set — Unified module configuration

Configure a module's persistent settings through a single `mu set` command.
Each module registers a setter at startup, so the available modules are whatever
the binary was built with.

```bash
# List the available modules
mu set
# ask, es, git, installer, svcreg, watch, wol
```

`set` is handled before the CLI parser, so it does not appear in `mu --help`;
`mu set` on its own prints the module list.

## ask

```bash
# Legacy flat fields (single provider). --config selects the file to write.
mu set ask flat --api-key sk-xxx --base-url https://api.deepseek.com --model deepseek-chat
mu set ask flat --search-key <brave-search-key>

# Named providers (with fallback order)
mu set ask provider add --name deepseek --base-url https://api.deepseek.com --api-key sk-xxx
mu set ask provider add --name backup --base-url https://api.deepseek.com --api-key sk-yyy
mu set ask provider set deepseek,backup   # first is primary, rest are fallbacks
mu set ask provider list
mu set ask module --provider deepseek,backup --search-key <brave-search-key>
mu set ask provider rm backup

# Any of the above, against a custom file (the reader honours it too)
mu set ask provider add --name deepseek --base-url https://api.deepseek.com --api-key sk-xxx --config /etc/mu/ask.json
mu ask --config /etc/mu/ask.json "how does the downloader resume?" 
```

## es

```bash
mu set es --host http://localhost:9200 --username elastic --password secret
```

## git

```bash
# LLM providers are shared by commit and review
mu set git provider add --name deepseek --base-url https://api.deepseek.com --api-key sk-xxx
mu set git provider add --name backup --base-url https://api.deepseek.com --api-key sk-yyy
mu set git provider list

# Per-module settings (the provider must exist first)
mu set git commit --provider deepseek --lang en
mu set git review --provider deepseek --lang en \
  --reviews-dir ~/.cache/mu/git_reviews \
  --scip-version go=v0.3.0
mu set git review --scip-version-rm go

# Removing a provider is refused while commit/review still point at it
mu set git commit --provider backup
mu set git review --provider backup --lang en
mu set git provider rm --name deepseek

# Custom config file: same flag on every subcommand, and on the readers
mu set git commit --provider deepseek --lang en --config /etc/mu/git-config.json
mu git commit --config /etc/mu/git-config.json
mu git review --config /etc/mu/git-config.json
```

## installer

```bash
mu set installer --token <github-token>   # raise the GitHub API rate limit
mu set installer --unset                  # remove the stored token
```

## svcreg

```bash
mu set svcreg --host 0.0.0.0 --port 30100 --db-path ~/.config/mu/svcreg.db
```

## watch

```bash
mu set watch --git-user myuser --git-password ghp_xxx
```

## wol

```bash
mu set wol --server http://192.168.1.100:8080 --port 8080 \
  --interface br-lan --hostname my-machine --token secret \
  --db-path ~/.config/mu/bolt.db
```

## Config files

Every module keeps its settings in `~/.config/mu/<module>-config.json` (the
`watch` module uses `~/.config/mu/watch.json`), and secret-bearing files are
written with mode `0600`. Use `mu set <module> --help` for the exact flags and
defaults.

`--config <path>` selects the file to use, and it applies to readers as well as
writers: `mu set ask provider add … --config /etc/mu/ask.json` writes that file
and `mu ask --config /etc/mu/ask.json …` reads it, so a custom path round-trips
without touching `~/.config/mu`. The same holds for the git commands
(`mu set git …`, `mu git commit`, `mu git review`) and for `mu watch git`.
