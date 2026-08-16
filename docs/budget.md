# budget — Query LLM API usage and balance

Track API usage and account balance across multiple LLM providers (DeepSeek, OpenRouter, Aliyun).
Supports both CLI and web UI, integrated into the gateway.

```bash
# Query all configured providers
mu budget balance

# Query a single provider
mu budget balance -p deepseek
mu budget balance -p openrouter -k sk-or-v1-xxx

# Query Aliyun balance + resource packages
mu budget balance -p aliyun

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

Debug logging can be enabled with `"debug_log": true` — writes to `~/.config/mu/budget.log`.
