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

All services are optional — if a config file is missing (mock), the corresponding route is
skipped with a warning and the rest of the gateway starts normally.
