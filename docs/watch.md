# watch — Watch resources for changes

Monitor file systems and git remotes for changes, with real-time event output.

```bash
# Watch a directory for file changes (every 5s)
mu watch file ./src

# Custom interval with glob filtering
mu watch file . --interval 2s --include "*.go" --exclude "vendor/*"

# Watch git remote for upstream updates (every 60s)
mu watch git . --interval 30s --branch main
```

```
$ mu watch file ./src --interval 2s --include "*.go"
Watching /home/user/src for changes (interval: 2s)...
[2026-07-07 14:00:00] ADDED    src/main.go
[2026-07-07 14:00:05] MODIFIED src/utils.go
[2026-07-07 14:00:10] DELETED  src/old.go
```

Git authentication (env vars take priority over config):

```bash
export GIT_AUTH_USER="myuser"
export GIT_AUTH_PASS="ghp_xxx"
mu watch git . --interval 60s
```

Or configure in `~/.config/mu/watch.json`:

```json
{
  "git_auth": {
    "username": "myuser",
    "password": "ghp_xxx"
  }
}
```
