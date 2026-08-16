# svcreg — Service Registry (ServiceCenter-compatible)

A lightweight RESTful server compatible with Apache ServiceComb ServiceCenter protocol.
Acts as a drop-in replacement for `service-center` — Java Chassis / Spring Cloud Huawei
clients can point `discovery.address` to `mu svcreg` and continue working unchanged.

Includes a Svelte 5 web dashboard (dashboard, services, instances, admin) with integrated
server lifecycle management.

```bash
# Start API-only server (bare, no frontend)
mu svcreg serve

# Start API + web dashboard (same port)
mu svcreg serve --web

# Start standalone web frontend (connects to a remote serve)
mu svcreg frontend --server http://192.168.1.10:30100 --port 30101

# Show server status
mu svcreg status

# List registered services
mu svcreg list services
mu svcreg list services --environment development

# List instances (single service or all)
mu svcreg list instances --service-id <id>
mu svcreg list instances --all
mu svcreg list instances --all --environment production
```

## Web Dashboard

The web frontend (served at `serve --web` or standalone `frontend`) provides four tabs:

| Tab | Features |
|-----|----------|
| Dashboard | Server status, service/instance counts |
| Services | Filter by environment, expand for instance detail, delete |
| Instances | All instances across services |
| Admin | Start/stop serve subprocess, config port/host/DB path, live logs, independent process group option |

The Admin tab persists server state across restarts via PID file recovery.

## Configuration

Server settings persisted in `~/.config/mu/svcreg-config.json`:

```json
{
    "host": "0.0.0.0",
    "port": 30100,
    "db_path": "~/.config/mu/svcreg.db"
}
```

CLI flags (`--host`, `--port`, `--db-path`) override the config file.

## Supported API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v4/{project}/registry/version` | Server version + listen address |
| `GET` | `/v4/{project}/registry/health` | Cluster health |
| `GET` | `/v4/{project}/registry/existence` | Check service/schema existence |
| `POST` | `/v4/{project}/registry/microservices` | Register service |
| `GET` | `/v4/{project}/registry/microservices` | List services |
| `GET` | `/v4/{project}/registry/microservices/{id}` | Get service detail |
| `PUT` | `.../{id}/properties` | Update service properties |
| `DELETE` | `.../{id}` | Delete service (cascades instances + schemas) |
| `POST` | `.../{serviceId}/instances` | Register instance |
| `GET` | `.../{serviceId}/instances` | List instances |
| `DELETE` | `.../{serviceId}/instances/{id}` | Unregister instance |
| `PUT` | `.../instances/{id}/properties` | Update instance properties |
| `PUT` | `.../instances/{id}/status?value=` | Update instance status |
| `PUT` | `.../instances/{id}/heartbeat` | Single heartbeat |
| `PUT` | `/v4/{project}/registry/heartbeats` | Batch heartbeat |
| `GET` | `/v4/{project}/registry/instances` | Find instances |
| `POST` | `/v4/{project}/registry/instances/action?type=query` | Batch find (SDK fallback) |
| `GET` | `.../watcher` | WebSocket instance watch |
| `POST/GET/DELETE` | Tags + Schemas | Tag and schema management |

## Storage

Uses BoltDB (embedded), no external dependencies. Instance TTL defaults to
30s interval × 3 retries (90s). Heartbeat via REST `PUT` extends the lease.

Logs every request with method, path, status code, and duration.
