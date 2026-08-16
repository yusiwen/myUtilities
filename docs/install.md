# install — Install binaries from GitHub releases

```bash
mu install owner/repo --move
```

Install from a specific release tag, or search by program name:

```bash
mu install owner/repo@v1.2.0     # specific tag
mu install jq                     # auto-search GitHub for "jq"
```

Private repos and rate-limit avoidance via a GitHub token:

```bash
mu install owner/private-repo --token ghp_xxx
mu set installer --token ghp_xxx        # persist in ~/.config/mu/installer-config.json
mu set installer --unset                # remove the stored token
```
