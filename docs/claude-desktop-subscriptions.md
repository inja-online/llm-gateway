# Claude app with subscription OAuth

**Last updated:** 2026-08-28

> **Docs site (recommended):** [Claude app + subscriptions](https://inja-online.github.io/llm-gateway/guides/claude-desktop-subscriptions/) — 3P inference, import JSON, Claude Code inside the app, troubleshooting.

Point the **Claude desktop application** (macOS/Windows) at the local gateway. Chat in the app uses **Developer → Configure Third-Party Inference** (not `ANTHROPIC_BASE_URL`). Claude Code inside the app / the `claude` CLI still uses Anthropic env vars.

```bash
llm-gateway helpers install
eval "$(llm-gateway helpers source)"
llm-gateway auth import grok    # and/or chatgpt, claude, gemini
export KEY=local-dev
cc-gateway-up

CC_MODEL=grok-4.6 apps-write-claude-desktop
# Import ~/.config/inja-gateway/claude-desktop-3p.json in:
#   Help → Troubleshooting → Enable Developer Mode
#   Developer → Configure Third-Party Inference… → Import → Apply locally
```

| Field | Value |
|-------|--------|
| Inference provider | Gateway |
| Gateway base URL | `https://127.0.0.1:8787` |
| API key | `local-dev` |
| Auth scheme | x-api-key |

Helpers **merge** `env` into `claude_desktop_config.json` (MCP / Cowork prefs kept). Rollback: `apps-use-default`.

CLI wrappers need `#!/bin/zsh` (or bash). `/bin/sh` rejects `cc-gateway-up()`. Unset `ANTHROPIC_API_KEY` and keep `ANTHROPIC_AUTH_TOKEN=local-dev` to avoid Claude Code’s dual-key warning.

Related: [claude-code-multi.md](claude-code-multi.md) · [codex-subscriptions.md](codex-subscriptions.md)
