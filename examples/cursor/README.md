# Cursor IDE + subscription gateway

Use the **same** Inja LLM Gateway (ChatGPT / Claude / SuperGrok OAuth) from **Cursor**, not only Claude Code.

Cursor Settings only accept configuration through the **GUI** (OpenAI API key + Override OpenAI Base URL). Helpers print the exact values to paste.

## Quick path

```bash
# 1) Subscriptions logged in (once)
./llm-gateway auth login chatgpt   # and/or claude, grok
./llm-gateway auth import grok     # if you use Grok CLI

# 2) HTTPS gateway in background
source examples/shell/claude-code-helpers.sh
source examples/shell/cursor-helpers.sh
export KEY=local-dev
cc-gateway-up

# 3) Print Cursor Settings fields
cursor-setup
```

## Cursor Settings → Models

| Field | Value |
|-------|--------|
| **OpenAI API Key** | `local-dev` (or your edge key) |
| **Override OpenAI Base URL** | `https://127.0.0.1:8787/v1` |

**Must include `/v1`.** Cursor calls `{base}/chat/completions` and may call `{base}/responses` in Agent mode.

### Models to add (custom)

Use gateway **aliases** (from `claude-code-subscriptions.yaml`):

| Cursor model name | Upstream |
|-------------------|----------|
| `gpt` / `gpt-mini` / `o3` | ChatGPT subscription |
| `grok-4.5` / `composer-2.5` | SuperGrok |
| `sonnet` / `opus` / `haiku` | Claude subscription |

Or full ids: `chatgpt/gpt-5.1`, `xai/grok-4.5`, `anthropic/claude-sonnet-4-20250514`.

1. **Add Model** for each name you want in the picker.  
2. **Verify** if the button is available.  
3. Select the model in Chat / Agent.

## Combos

Same idea as Claude Code — only enable models for the subscriptions you logged into:

- **GPT only** → add `gpt`, `o3`, `gpt-mini`  
- **Grok only** → add `grok-4.5`, `composer-2.5`  
- **GPT + Grok** → both  
- **+ Claude** → also `sonnet` / `opus` / `haiku` after `auth login claude`

You do **not** need separate Cursor “Anthropic API key” for Claude-through-gateway: use the OpenAI override and a Claude **model id** so traffic is `POST /v1/chat/completions` with `model=sonnet` (gateway routes + translates/passthroughs).

## TLS

`cc-gateway-up` serves **HTTPS** with certs from `examples/scripts/gen-localhost-tls.sh`.

- Prefer **mkcert** (`brew install mkcert && mkcert -install`) so Cursor trusts the cert.  
- Self-signed: helpers set `NODE_EXTRA_CA_CERTS` in the shell, but Cursor may still need mkcert system trust.

## Caveats (Cursor product limits)

| Topic | Notes |
|-------|--------|
| Settings are GUI-only | Cursor stores keys in secure storage; scripts cannot write them for you |
| Agent vs Ask | Agent may use **Responses** (`/v1/responses`); this gateway supports that for openai / openai_compat (ChatGPT, xAI) |
| Multimodal BYOK | Some Cursor versions send image requests to `api.openai.com` despite override — text/agent is the supported path |
| Built-in Cursor models | Unrelated to the gateway; disable or ignore them if you only want subscription routing |

## Verify from the terminal

```bash
cursor-verify    # GET /v1/models through the gateway
curl -sk https://127.0.0.1:8787/healthz
```

## Related

- Full docs site: [Cursor + subscriptions](https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/)
- Claude Code same gateway: [Claude Code + subscriptions](https://inja-online.github.io/llm-gateway/guides/claude-code-subscriptions/)
- Config: [`examples/configs/claude-code-subscriptions.yaml`](../configs/claude-code-subscriptions.yaml)
