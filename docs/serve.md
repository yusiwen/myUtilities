# serve — Static file server

Start an HTTP static file server for a local directory. Useful for previewing static sites or sharing files over LAN.

```bash
# Serve current directory on port 8080
mu serve

# Serve a specific directory on a custom port
mu serve ./dist --port 3000

# Enable CORS for cross-origin requests
mu serve --cors

# Log requests to stderr
mu serve -v
```

```
$ mu serve ./dist --port 3000 --cors
Serving /home/user/project/dist on http://localhost:3000
```
