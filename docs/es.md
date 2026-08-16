# es — Elasticsearch query tool

Query and explore Elasticsearch indices through a web UI. Connection settings are
persisted in `~/.config/mu/es-config.json`.

```bash
# Configure the ES connection (or use mu set es)
mu es set host http://localhost:9200
mu es set user elastic
mu es set password my-password

# Serve web UI (standalone)
mu es serve --port 8084
```

The web UI provides:
- **Status** — connection health check against the configured host
- **Indices** — browse index list with document counts
- **Search** — run arbitrary ES queries and view results

Also available in the gateway at `/es/`. Connection info is masked in config
displays (password never shown in plaintext).
