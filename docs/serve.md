# serve — Static file server and single-file sharing

`mu serve` serves a directory tree, or a single file with a QR code that a phone
on the same LAN can scan to download it.

## Directory mode

```bash
# Serve the current directory on port 8080
mu serve

# Serve a specific directory on a custom port
mu serve ./dist --port 3000

# Enable CORS for cross-origin requests
mu serve ./dist --cors

# Log requests to stderr
mu serve ./dist --verbose
```

```
$ mu serve ./dist --port 3000
Serving /home/user/project/dist on http://localhost:3000
  URL:   http://192.168.1.20:3000/
```

## Single-file mode

Pass a regular file instead of a directory. The file is served at `/` (short URL,
which keeps the QR code small) and again at `/<name>` for links copied by hand.

```bash
mu serve ./release.zip
```

```
Serving /home/user/release.zip (128.4 MiB)
  URL:   http://192.168.1.20:8080/
  same:  http://192.168.1.20:8080/release.zip
  local: http://localhost:8080/
Press Ctrl+C to stop.
Scan with your phone:
<QR code>
```

The QR code is drawn with an explicit black-and-white canvas, so it scans the
same on light and dark terminal themes. It is only drawn when stdout is a
terminal and the window is wide enough for it; pipes, log files and `--no-qr`
get plain text output.

| Request | Response |
|---|---|
| `GET /` | the file, `Content-Disposition: attachment` |
| `GET /<name>` | the same file |
| `HEAD` | headers only |
| `Range` | `206 Partial Content`, so downloads resume |
| any other path | `404` (also for a wrong or missing token) |
| any other method | `405` |
| after `--expire` | `410 Gone` |

With `--token`, the same paths apply under `/<token>`; `/` and `/<name>` on
their own answer `404`.

## Temporary share links

The three single-file options make a link that stops working when you want it
to. They combine freely.

```bash
# Unguessable path: only someone with the link can download
mu serve ./release.zip --random-token

# Fixed token, one download, then the process exits
mu serve ./release.zip --token=v1.2.3 --once

# Link dies after ten minutes
mu serve ./release.zip --expire 10m
```

```
Serving /home/user/release.zip (128.4 MiB)
  URL:   http://192.168.1.20:8080/ksaduzq7WiuPpIor
  same:  http://192.168.1.20:8080/ksaduzq7WiuPpIor/release.zip
  local: http://localhost:8080/ksaduzq7WiuPpIor
Press Ctrl+C to stop.
  Link requires the token; other paths answer 404.
  Single use: the server stops after one complete download.
  Expires at 11:42:05 (in 10m0s).
```

| Option | Behaviour |
|---|---|
| `--token <value>` | The token must be the first path segment. Every other path answers `404`, so the file cannot be discovered by scanning the service. Only letters, digits and `- . _ ~` are accepted; the token is compared in constant time. |
| `--random-token` | Generates a 16-character token (96 bits) instead of taking one from the command line. Mutually exclusive with `--token`. |
| `--once` | The process shuts down after the first transfer that delivered the whole file. A partial `Range` request or a client that disconnects does not count, so a failed download can be retried. |
| `--expire <duration>` | After the duration, requests answer `410 Gone` and the process shuts down, giving in-flight transfers five seconds to finish. |

Notes:

- All three only apply to a single file; using them with a directory is an
  error rather than a silent no-op.
- `--verbose` redacts the token from the request log.
- The token is part of the URL, so anyone who sees the URL can download until
  `--once`/`--expire` ends the link. It is not user authentication.

## Flags

| Flag | Default | Description |
|---|---|---|
| `PATH` | `.` | Directory or single file to serve |
| `-p`, `--port` | `8080` | Port to listen on (`0` picks a free port) |
| `--host` | all interfaces | Address to bind to |
| `--iface` | auto | Interface whose address is advertised in the QR code |
| `--token` | none | Token required as the URL path (single file only) |
| `--random-token` | off | Generate the token (single file only) |
| `--once` | off | Stop after the first complete download (single file only) |
| `--expire` | `0` (never) | Link lifetime, e.g. `10m` (single file only) |
| `--qr` / `--no-qr` | on | Draw the QR code when stdout is a terminal |
| `--cors` | off | Add CORS headers for all origins |
| `--verbose` | off | Log every request to stderr |

## How the LAN address is chosen

The advertised address is the one the kernel would use to reach the internet: a
UDP "connection" is opened to a public address to ask for the source address,
which sends no traffic. That is normally the address a phone on the same Wi-Fi
reaches.

Other addresses are listed as `alt:` lines (container bridges, VPNs and tunnels
are ordered last and marked by interface name). Use `--iface` to pin the
advertised interface, or `--host` to bind and advertise one specific address.
Binding to a loopback address prints a warning: the server then works, but no
phone can reach it.

## Notes

- Downloads are not cut off by a wall-clock timeout. The server keeps the header
  timeout (slowloris defence) and the idle timeout, but leaves the transfer
  itself unbounded, so a multi-gigabyte file on a slow LAN completes.
- There is no authentication: anyone who can reach the port can download the
  file. Bind to a specific address with `--host`, or serve only on a trusted
  network.
- If the terminal is too narrow for the QR code, widen the window or write a PNG
  with `mu qrcode gen -o qr.png "http://<ip>:<port>/"` and show that instead.
