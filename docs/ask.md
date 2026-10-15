# ask — Ask LLM questions with optional web search

Sends a question to an OpenAI-compatible LLM API and returns a concise answer with reference URLs. Optionally fetches web search results via Brave Search API for up-to-date answers with source citations.

```bash
# Ask a question (uses LLM knowledge)
mu ask "What is a goroutine in Go?"

# With web search for real-time information
mu ask --search "What is WebAssembly?"
mu ask -s "Rust vs Go 2025 comparison"

# Chinese answer
mu ask --lang cn "什么是 WebAssembly？"

# Pipe input
echo "Explain TCP handshake" | mu ask

# Debug mode
mu ask --model gpt-4o --verbose "How does TLS work?"
```

## Provider fallback

You can configure multiple providers and the ask module will try them in order until one succeeds:

```bash
# Define named providers
mu set ask provider add --name fast --base-url "https://fast.example.com/v1" --api-key "sk-xxx"
mu set ask provider add --name backup --base-url "https://backup.example.com/v1" --api-key "sk-xxx"

# Set fallback chain for the ask module (comma-separated)
mu set ask provider set fast,backup

# Alternative: set it via the module subcommand
mu set ask module --provider fast,backup

# Use a specific provider (overrides config)
mu ask --provider fast "What's happening?"

# List configured providers and module references
mu set ask provider list

# Remove a provider
mu set ask provider rm fast
```

## Configuration

Configuration file: `~/.config/mu/ask-config.json` (override with
`mu ask --config <path>`, and with `mu set ask … --config <path>`).
```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-xxx",
  "model": "gpt-4o-mini",
  "search_api_key": "BSA-xxx",
  "providers": [
    { "name": "fast", "base_url": "https://fast.example.com/v1", "api_key": "sk-xxx", "model": "gpt-4o-mini" },
    { "name": "backup", "base_url": "https://backup.example.com/v1", "api_key": "sk-xxx", "model": "gpt-4o-mini" }
  ],
  "provider": ["fast", "backup"]
}
```

All settings can also be set via environment variables (`OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL`, `BRAVE_SEARCH_API_KEY`) or CLI flags.

## Shared providers

Providers can also live in the shared catalog `~/.config/mu/llm-config.json`, so one
endpoint definition serves `mu ask`, `mu git commit` and `mu git review`:

```bash
mu set llm provider add --name deepseek --base-url https://api.deepseek.com/v1 \
  --api-key sk-xxx --model deepseek-chat
mu set ask module --provider deepseek
mu set llm provider list       # shows the modules depending on the catalog
```

Precedence, for a name defined in both files: the `ask-config.json` entry wins
**field by field** — a non-empty field overrides the shared value and an empty field
keeps it, so this module can change only the `model` of a shared endpoint:

```bash
mu set ask provider add --name deepseek --model deepseek-chat   # keep the shared endpoint and key
```

A config without `providers` uses the shared catalog unchanged, and nothing in the
shared file is copied into `ask-config.json` (loading never rewrites a config file).

`--llm-config <path>` points every reader and `mu set llm …` at a different catalog
file, matching the `--config` convention.

