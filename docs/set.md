# set — Unified module configuration

Configure any module's persistent settings via a single `mu set` command.
Each module implements the `ModuleSetter` interface and registers itself
at startup.

```bash
# List available modules
mu set

# Update ask config
mu set ask --config-api-key sk-xxx --config-base-url https://api.deepseek.com

# Update git commit config
mu set commit --config-model deepseek-v4-flash --config-api-key sk-xxx

# Update ES connection
mu set es --config-host http://localhost:9200 --config-username elastic

# Update service registry
mu set svcreg --config-host 0.0.0.0 --config-port 30100

# Update WOL settings
mu set wol --config-interface br-lan --config-port 8080 --config-token secret

# Update watch auth
mu set watch --config-git-user myuser --config-git-password ghp_xxx

# All flags support --config <path> to use a custom file
mu set es --config-host https://es.example.com --config /etc/mu/es-config.json
```

Available modules: ask, es, git, svcreg, watch, wol.
