# Windsurf / Cascade → Inja LLM Gateway

Same subscriptions as Claude Code / Cursor. Windsurf talks **OpenAI-compatible** HTTP.

## Values

| Field | Value |
|-------|--------|
| Base URL / OpenAI endpoint | `https://127.0.0.1:8787/v1` |
| API key | `local-dev` (gateway edge key, not a Platform `sk-`) |
| Models | `gpt`, `sol`, `terra`, `luna`, `grok-4.5`, `composer-2.5`, `sonnet`, `opus`, … |

## Steps

1. `cc-gateway-up` (HTTPS gateway with subscription OAuth).
2. Windsurf → **Settings** → AI / Models / Providers.
3. Add or edit an **OpenAI-compatible** / custom provider with the base URL and key above.
4. Add custom model names that match gateway aliases.
5. If TLS errors: `mkcert -install` or set `NODE_EXTRA_CA_CERTS` before launching Windsurf.

Exact menu labels vary by Windsurf version; search settings for “OpenAI”, “Base URL”, or “Custom”.
