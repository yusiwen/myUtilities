# metrics — Time-series metrics collection

Collect host metrics (CPU, memory, disk, load) as a long-running agent, store them
in a built-in time-series DB (BoltDB), and query them via CLI or HTTP API.

```bash
# Start the metrics server (HTTP API on port 8096), optionally with built-in agent
mu metrics serve --port 8096 --agent --interval 30s

# Run a standalone agent that reports to a remote server
mu metrics agent --server http://metrics-host:8096 --interval 30s

# Query stored metrics (names: cpu.used.percent, memory.used.percent, load.1m, etc.)
mu metrics query cpu.used.percent --last 1h --format table
mu metrics query --list                 # list all metric names
mu metrics query load.1m --tags host=myhost --format json

# Manually compact / expire old data
mu metrics compact --retention 30d
```

The HTTP server exposes JSON APIs under `/api/metrics` (list, query, write,
compact) for the agent and CLI clients.
