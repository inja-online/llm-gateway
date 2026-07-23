# Cursor IDE + subscription gateway

Use the **same** Inja LLM Gateway (ChatGPT / Claude / SuperGrok OAuth) from **Cursor**, **alongside** Cursor’s built-in models.

## Coexistence (what you asked for)

| Model in Cursor picker | Who serves it | Billing |
|------------------------|---------------|---------|
| **Claude Fable 5** (built-in) | Cursor | Cursor plan / Other Models pool |
| **`claude/fable-5`** (custom) | llm-gateway → your Claude sub | Your Claude subscription via gateway |
| **Composer 2.5** (built-in) | Cursor | Cursor Models pool |
| **`grok/composer-2.5`** (custom) | llm-gateway → SuperGrok | SuperGrok via gateway |
| **GPT-5.6 Terra** (if shown by Cursor) | Cursor | Cursor |
| **`chatgpt/terra`** (custom) | llm-gateway → ChatGPT sub | ChatGPT via gateway |

**Yes, this is possible:** leave Cursor’s models enabled, set OpenAI key + **Override OpenAI Base URL** only for **custom** OpenAI-compatible models, and **Add Model** with **prefixed** names so nothing collides with Cursor’s labels.

```text
Claude Fable 5     ← Cursor (keep it)
claude/fable-5     ← gateway (add it)
```

## Quick path (automated)

### Release binary

```bash
llm-gateway helpers install
eval "$(llm-gateway helpers source)"
llm-gateway auth login chatgpt   # and/or claude, grok
export KEY=local-dev
cc-gateway-up

# Fully quit Cursor first (Cmd+Q)
cursor-apply          # writes openAIBaseUrl + merges custom models into state.vscdb
cursor-status

# Reopen Cursor → Settings → Models → paste OpenAI API Key once: local-dev
cc-gateway-logs -f    # confirm traffic when you pick claude/fable-5 etc.
```

### Git checkout

```bash
./llm-gateway auth login chatgpt   # and/or claude, grok
./llm-gateway auth import grok

source examples/shell/claude-code-helpers.sh
source examples/shell/cursor-helpers.sh
export KEY=local-dev
cc-gateway-up

cursor-setup
cursor-apply          # quit Cursor first
cursor-status
```

Manual alternative: `cursor-models` then **Add Model** for each line.  
Copy-paste list: [`models-to-add.txt`](models-to-add.txt).

| Command | What it does |
|---------|----------------|
| `cursor-apply` | Backup + set base URL + merge `claude/…` `chatgpt/…` `grok/…` `inja/…` into Cursor |
| `cursor-status` | Show stored base URL and `userAddedModels` |
| `cursor-rollback` | Restore last backup |
| `cursor-verify` | Probe gateway catalog |
| `cc-gateway-logs -f` | HTTP + usage lines for gateway-routed traffic |

**Cannot automate:** OpenAI API key (Electron encrypted storage) — paste once in the UI.

## Cursor Settings → Models

| Field | Value |
|-------|--------|
| **OpenAI API Key** | `local-dev` (gateway edge key) |
| **Override OpenAI Base URL** | `https://127.0.0.1:8787/v1` |

**Must include `/v1`.**

### What to leave alone

- Cursor Models (Composer 2.5, Cursor Grok, …)
- Anthropic / OpenAI / Google models that Cursor ships (including **Claude Fable 5**)

### What to add (custom)

Prefer **prefixed** ids (gateway aliases):

| Add Model name | Routes to |
|----------------|-----------|
| `claude/fable-5` | Claude Fable 5 via your Claude sub |
| `claude/sonnet-5` | Claude Sonnet 5 via gateway |
| `claude/opus` | Claude Opus 4.8 via gateway |
| `chatgpt/terra` / `chatgpt/sol` / `chatgpt/luna` | GPT-5.6 via ChatGPT sub |
| `grok/4.5` | SuperGrok Grok 4.5 |
| `grok/composer-2.5` | SuperGrok Build (`grok-build-0.1`) |
| `inja/…` | Same targets, explicit “gateway” namespace |

Full aliases: [`examples/configs/claude-code-subscriptions.yaml`](../configs/claude-code-subscriptions.yaml) (also installed as `~/.config/inja-gateway/claude-code-subscriptions.yaml`).

### Avoid

Do **not** add bare names like `fable` or `fable-5` if they make the picker ambiguous next to Cursor’s **Claude Fable 5**. Use `claude/fable-5` or `inja/fable-5` instead.

## How the two paths work

```
  Cursor built-in model  ──► Cursor cloud / provider (unchanged)
  Custom model (claude/fable-5)
       │  OpenAI API key + Override Base URL
       ▼
  https://127.0.0.1:8787/v1/chat/completions  (or /responses)
       ▼
  llm-gateway  ──► Claude / ChatGPT / SuperGrok subscriptions
```

## TLS

Prefer `mkcert -install` + `./examples/scripts/gen-localhost-tls.sh` so Electron/Cursor trusts local HTTPS.

## Verify

```bash
cursor-verify
curl -sk https://127.0.0.1:8787/healthz
cc-gateway-logs --usage
```

## Related

- Docs site: [Cursor + subscriptions](https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/)
- Any app: [App integrations](https://inja-online.github.io/llm-gateway/guides/app-integrations/)
- Claude Code: [Claude Code + subscriptions](https://inja-online.github.io/llm-gateway/guides/claude-code-subscriptions/)
