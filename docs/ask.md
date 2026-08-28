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

Configuration file: `~/.config/mu/ask-config.json`

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
