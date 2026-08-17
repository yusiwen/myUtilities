# gateway — Unified service portal

Serves multiple mu services under a single HTTP server with a landing page.

```bash
mu gateway --port 8080
mu gateway --config-dir /etc/mu    # custom config directory
```

By default, module configs are read from `~/.config/mu/`:

```
~/.config/mu/
├── wol-config.json
├── es-config.json
├── mock-config.json     (optional, auto-created on first start)
├── budget-config.json
├── ask-config.json
├── git-config.json
├── svcreg-config.json
└── watch.json
```

| Route | Service | Description |
|---|---|---|---|
| `/` | Landing page | Card-based navigation to all services |
| `/wol/*` | Wake-on-LAN | WOL management frontend and API |
| `/es/*` | Elasticsearch | ES query frontend and API |
| `/mock/__admin/*` | Mock Dynamic | Dynamic mock endpoint management |
| `/qrcode/` | QR Code | QR code generator web UI |
| `/jarinfo/` | JAR Analyzer | JAR file analysis web UI |
| `/crypto/` | Crypto | Encrypt, decrypt, passwords, JWT, encode/decode |
| `/diff/` | Diff | Side-by-side text comparison |
| `/k8s/` | K8s | Kubernetes Secret YAML generator and decoder |
| `/misc/` | Misc | JSON, UUID, timestamp, hash, tracker list tools |
| `/network/` | Network | DNS, DIG, and WHOIS query tools |
| `/svcreg/` | Service Registry | Register and discover microservices |
| `/budget/` | API Budget | Track LLM API balance across providers |
| `/metrics/` | Metrics | System monitoring charts (read-only proxy to `mu metrics serve`) |

All services are optional — if a config file is missing (mock), the corresponding route is
skipped with a warning and the rest of the gateway starts normally.

The metrics route proxies read-only to a running `mu metrics serve` backend
(default `http://localhost:8096`, override with `--metrics-server` or `MU_METRICS_SERVER`).
Only `GET /api/metrics` and `GET /api/metrics/{name}` are forwarded; the
write/compact endpoints are never exposed through the gateway.

## Managed metrics server

By default the gateway manages its own `mu metrics serve` subprocess instead of
relying on an external one:

```bash
mu gateway --port 8080                      # auto-starts a managed server on :8096
mu gateway --metrics-port 19096             # different port
mu gateway --metrics-auto-start=false       # manage but don't auto-start
mu gateway --metrics-manage=false           # pure proxy to --metrics-server
```

- The subprocess is spawned as `mu metrics serve --port <p> --agent` (same
  binary as the gateway) and follows the gateway's lifecycle: on SIGTERM/SIGINT
  the gateway stops it first; on a hard SIGKILL the child dies with the parent
  (Linux `PDEATHSIG`).
- **Port probing:** if the port is already served by a `mu metrics` server the
  gateway detects it and enters `external` mode (no subprocess spawned, proxy
  direct). If the port is occupied by a non-metrics service the gateway reports
  an `error` state and the dashboard shows it.
- The metrics page shows an admin control bar (status, pid, uptime, recent
  logs) with Start/Stop/Restart buttons, backed by:
  - `GET  /api/metrics/admin/status`
  - `POST /api/metrics/admin/start`
  - `POST /api/metrics/admin/stop`
  - `POST /api/metrics/admin/restart`
