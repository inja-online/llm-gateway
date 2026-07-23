# Use your subscriptions in any app

One local **Inja LLM Gateway** + your **ChatGPT / Claude / SuperGrok** logins can back many clients.

```
  Claude Desktop · Claude Code · Cursor · ChatGPT/Codex Desktop
  Continue · Cline · Aider · Windsurf · SDKs / curl
                                      │
                                      ▼
                         https://127.0.0.1:8787
                         (subscription OAuth on server)
```

## 0. Shared setup (once)

```bash
go build -o llm-gateway ./cmd/gateway
./llm-gateway auth login chatgpt   # and/or claude, grok
./llm-gateway auth import grok     # recommended for SuperGrok

source examples/shell/claude-code-helpers.sh
export KEY=local-dev
cc-gateway-up                      # HTTPS background gateway

source examples/shell/apps-helpers.sh
apps-setup                         # print every integration
```

Dialects the gateway speaks:

| Dialect | Paths | Typical apps |
|---------|--------|----------------|
| **Anthropic** | `POST /v1/messages` | Claude Desktop, Claude Code |
| **OpenAI** | `POST /v1/chat/completions`, `/v1/responses`, `/v1/models` | Cursor, Codex/ChatGPT coding, Continue, Cline, Aider, Windsurf, most “OpenAI-compatible” apps |

Base URLs:

| Use | URL |
|-----|-----|
| Anthropic clients | `https://127.0.0.1:8787` (**no** trailing `/v1` for Claude Code/Desktop env) |
| OpenAI clients | `https://127.0.0.1:8787/v1` (**with** `/v1`) |
| API key / edge key | `local-dev` (or your `KEY`) — not a Platform `sk-` |

Live models on your account:

```bash
curl -sk -H "Authorization: Bearer local-dev" \
  'https://127.0.0.1:8787/v1/models?live=1' | jq -r '.data[].id' | sort
```

## Per-app templates & helpers

| App | Template / helper |
|-----|-------------------|
| Claude Desktop | `claude-desktop/` · `apps-claude-desktop` · `apps-write-claude-desktop` · `apps-write-claude-settings` |
| Claude Code | `claude-code-*` helpers · [guide](https://inja-online.github.io/llm-gateway/guides/claude-code-subscriptions/) |
| Cursor | `cursor-helpers.sh` · [guide](https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/) |
| ChatGPT Desktop / Codex | `codex/config.toml` · `apps-codex` · `apps-write-codex` |
| Continue.dev | `continue/config.yaml` · `apps-continue` |
| Cline / Roo | `cline/vscode-settings.snippet.json` · `apps-cline` |
| Aider | `aider/aider.env` · `apps-aider` |
| Windsurf | `windsurf/settings.snippet.md` · `apps-windsurf` |
| Generic OpenAI | `generic/openai.env` |
| Generic Anthropic | `generic/anthropic.env` |

Full guide: [App integrations (docs site)](https://inja-online.github.io/llm-gateway/guides/app-integrations/).

## Quick matrix

| App | Wire | Config surface |
|-----|------|----------------|
| Claude Desktop | Anthropic | `claude_desktop_config.json` and/or `~/.claude/settings.json` `env` |
| Claude Code | Anthropic | `ANTHROPIC_BASE_URL` (+ combo scripts) |
| Cursor | OpenAI | Settings → Models override base URL |
| Codex / GPT coding | OpenAI | `~/.codex/config.toml` provider |
| Continue / Cline / Aider / Windsurf | OpenAI | each app’s OpenAI-compatible settings |
| SDKs | either | `OPENAI_*` or `ANTHROPIC_*` |

“Works perfectly” requires: gateway up, TLS trust, correct dialect URL, edge key, and a model id the gateway can route. Apps that **hardcode** first-party hosts with no override cannot be forced through the gateway.
