# wol — Wake-on-LAN HTTP server

Starts an HTTP server with a Svelte frontend and REST API for managing WOL aliases and tracking host status (boot/shutdown).

```bash
# Start server (interface name examples: br-lan on Linux, en0 on macOS, Ethernet0 on Windows)
mu wol serve en0 --port 8080

# List available network interfaces
mu wol interfaces
mu wol interfaces -v  # verbose output
```

## Configuration

WOL settings are persisted in `~/.config/mu/wol-config.json`.

```bash
# Set agent server URL (used by mu wol agent when no URL is given)
mu wol set server http://192.168.1.100:8080

# Set network interface for the WOL server
mu wol set interface br-lan

# Set HTTP server port
mu wol set port 8080

# Set BoltDB file path
mu wol set db-path ~/.config/mu/bolt.db

# Set API auth token
mu wol set token my-secret-token

# Set hostname for agent registration
mu wol set hostname my-machine
```

## Agent Notifications

Send boot/shutdown events or register this machine on the WOL server. The server URL can be given inline or set once via `mu wol set server` and omitted afterwards.

```bash
# Register this machine (stores hostname→MAC mapping)
mu wol agent --register http://192.168.1.100:8080

# Same, using server URL from config
mu wol agent --register

# Send boot notification
mu wol agent --boot http://192.168.1.100:8080

# Send shutdown notification (from config)
mu wol agent --shutdown
```

Flags `--register`, `--boot`, and `--shutdown` are mutually exclusive.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/register` | Register agent: store hostname→MAC mapping (JSON: `{"name":"<host>","mac":"<mac>"}`) |
| `GET` | `/api/aliases` | List all hostname→MAC mappings |
| `POST` | `/api/aliases` | Add/update alias (JSON: `{"name":"<host>","mac":"<mac>"}`) |
| `DELETE` | `/api/aliases/{name}` | Delete an alias |
| `POST` | `/api/wake/{hostname}` | Send WOL magic packet |
| `POST` | `/api/notify/{hostname}?type=boot` | Record boot notification and set status to "boot" |
| `POST` | `/api/notify/{hostname}?type=shutdown` | Record shutdown notification and set status to "shutdown" |
| `GET` | `/api/notify/{hostname}` | Query current status (boot/shutdown/unknown) and recent events |
| `GET` | `/` | Svelte frontend UI |

Hostname must conform to RFC 952/1123. MAC must be in `xx:xx:xx:xx:xx:xx` format.

## systemd Integration

Example oneshot service files are provided for sending boot/shutdown notifications automatically.

**Boot** — `wol-agent-boot.service`: fires after network is online, before user login. Edit `ExecStart` to match your server and hostname, then:

```bash
sudo cp wol-agent-boot.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now wol-agent-boot.service
```

**Shutdown** — `wol-agent-shutdown.service`: fires only on actual system halt/poweroff/reboot. It uses `DefaultDependencies=no` + `Before=shutdown.target` to ensure the network is still available when the notification is sent. Unlike `ExecStop` in a combined unit, it cannot be triggered by a manual `systemctl stop`. Edit `ExecStart`, then:

```bash
sudo cp wol-agent-shutdown.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable wol-agent-shutdown.service
```
