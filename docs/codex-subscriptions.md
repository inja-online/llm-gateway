# Codex / ChatGPT coding with subscription OAuth

**Last updated:** 2026-08-28

> **Docs site (recommended):** [Codex + subscriptions](https://inja-online.github.io/llm-gateway/guides/codex-subscriptions/) — `config.toml`, ChatGPT desktop coding, TLS, models.

Point **Codex CLI** and ChatGPT **coding / Codex** surfaces at the gateway. They read `~/.codex/config.toml`. The ChatGPT **chat** UI often ignores a custom provider.

```bash
llm-gateway helpers install
eval "$(llm-gateway helpers source)"
llm-gateway auth login chatgpt
export KEY=local-dev
cc-gateway-up

apps-write-codex
export INJA_GATEWAY_KEY=local-dev
export NODE_EXTRA_CA_CERTS="$HOME/.config/inja-gateway/certs/localhost.pem"
codex
# codex --model grok-4.6
```

```toml
model = "gpt"
model_provider = "inja"
openai_base_url = "https://127.0.0.1:8787/v1"

[model_providers.inja]
name = "Inja LLM Gateway"
base_url = "https://127.0.0.1:8787/v1"
env_key = "INJA_GATEWAY_KEY"
wire_api = "chat_completions"
```

Template: [`examples/apps/codex/config.toml`](https://github.com/inja-online/llm-gateway/blob/master/examples/apps/codex/config.toml). Rollback: `apps-use-default`. Prefer `mkcert -install`. GUI apps need `INJA_GATEWAY_KEY` in the environment they inherit (not only an interactive zshrc).

Related: [claude-desktop-subscriptions.md](claude-desktop-subscriptions.md) · [claude-code-multi.md](claude-code-multi.md)
