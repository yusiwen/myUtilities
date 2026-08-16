# proxy — Database proxy with failover

```bash
mu proxy db --port 1521 \
  --route-name primary --db-host 10.0.0.1 --db-port 1521 \
  --route-name standby --db-host 10.0.0.2 --db-port 1521
```
