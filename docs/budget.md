# budget — Query LLM API usage and balance

Track API usage and account balance across multiple LLM providers (DeepSeek, OpenRouter, Aliyun),
and check whether the DeepSeek service itself is operational.
Supports both CLI and web UI, integrated into the gateway.

```bash
# Query all configured providers
mu budget balance

# Query a single provider
mu budget balance -p deepseek
mu budget balance -p openrouter -k sk-or-v1-xxx

# Query Aliyun balance + resource packages
mu budget balance -p aliyun

# Check the DeepSeek service status (no API key required)
mu budget status

# Machine-readable output, and a non-zero exit when not fully operational
mu budget status --json
mu budget status --check

# Serve web UI (standalone)
mu budget serve --port 8095
```

Configuration at `~/.config/mu/budget-config.json`:

```json
{
  "providers": {
    "deepseek": {"api_key": "sk-xxx"},
    "openrouter": {"api_key": "sk-or-v1-xxx"},
    "aliyun": {
      "access_key_id": "LTAI5txxx",
      "access_key_secret": "xxx"
    }
  },
  "debug_log": false
}
```

The web UI (also available at `/budget/` in the gateway) displays balance cards for
each provider with real-time data. Aliyun cards additionally show resource package
details (CDN traffic, storage, CU packages) with remaining amounts and expiry dates.

Each provider card includes a **Top Up ↗** link that opens the provider's official
recharge page in a new tab. The default URL can be overridden via the optional
`"top_up_url"` field in the config:

```json
{
  "providers": {
    "deepseek": {
      "api_key": "sk-xxx",
      "top_up_url": "https://my-custom-portal.com/recharge"
    }
  }
}
```

| Provider | Auth Method | API Endpoint |
|----------|-------------|--------------|
| DeepSeek | `Bearer <API_KEY>` | `GET /user/balance` |
| OpenRouter | Management key → `GET /api/v1/credits`, fallback to `GET /api/v1/auth/key` | |
| Aliyun | AK/SK HMAC-SHA1 signature | `QueryAccountBalance` + `QueryResourcePackageInstances` |

API key resolution: `--key` flag → `budget-config.json → providers.<name>.api_key`.

## DeepSeek service status

`mu budget status` reads the official status widget, which needs no authentication:
`GET https://status.deepseek.com/api/widget/v1/summary.json`. The web UI renders the same
check as a card above the balance cards and re-polls it every `poll_after_seconds`
(30 s by default).

`overall.status` is mapped to a verdict and an exit-relevant `ok` flag:

| `overall.status` | Verdict | `ok` |
|---|---|---|
| `operational` | healthy | true |
| `degraded` | degraded | false |
| `partial_outage`, `full_outage` | outage | false |
| `maintenance` | maintenance | false |
| anything else | unknown | false |

The result also carries the ongoing incidents (phase, impact, latest message, affected
components) and any in-progress/scheduled maintenance, so a run explains *why* the
verdict is not healthy. Pass `--check` to turn a non-operational verdict into a non-zero
exit code for monitoring; `--json` prints the raw result.

Freshness is reported separately: the snapshot is marked `stale` once `generated_at` is
older than `max_stale_seconds` (120 s), because a stale feed cannot prove the current
state. Staleness never changes the verdict by itself — it is surfaced as a warning in
both the CLI and the web card.

`mu budget serve --status-url <url>` points the standalone server at a mirror or a
local fixture; the gateway always uses the official endpoint.

The HTTP endpoint `GET /api/budget/status` returns the same structure
(`ok`, `health`, `summary`, `fetched_at`, `cached`, `not_modified`, `validated_at`);
an upstream failure answers `502` with `{"ok": false, "error": "..."}`.
Responses are cached in memory for `poll_after_seconds` and revalidated with the
upstream `ETag` via `If-None-Match`, so a `304 Not Modified` reuses the cached
snapshot instead of re-downloading it.


Debug logging can be enabled with `"debug_log": true` — writes to `~/.config/mu/budget.log`.
