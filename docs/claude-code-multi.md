# Claude Code with subscription OAuth (ChatGPT + Claude + SuperGrok)

**Last updated:** 2026-07-23

> **Docs site (recommended):** [Claude Code with ChatGPT, Claude & SuperGrok](https://inja-online.github.io/llm-gateway/guides/claude-code-subscriptions/) — full public guide with combos, aliases, and troubleshooting.

Use **your own consumer subscriptions** (not API keys) through the gateway:

| Provider | Login command | Subscription |
|---|---|---|
| **ChatGPT** | `llm-gateway auth login chatgpt` | ChatGPT Plus / Pro / Team via **Codex OAuth** (PKCE) |
| **Claude** | `llm-gateway auth login claude` | Claude Pro / Max via **setup-token** / Claude Code login |
| **Grok** | `llm-gateway auth login grok` | **SuperGrok** or **X Premium+** via xAI **device-code OAuth** |

Claude Code still speaks **Anthropic Messages**. The gateway passthroughs Claude and **translates** to ChatGPT/xAI when you pick those model aliases.

```
  Claude Code  ──Anthropic──►  llm-gateway  ──┬── Claude (subscription OAuth bearer)
                                              ├── ChatGPT (Codex OAuth refresh)
                                              └── xAI Grok (device OAuth refresh)
```

## Security & ToS (read this)

- Log in only with **accounts you own**. Credentials are stored **locally** (`~/.config/inja-gateway/credentials.json`, mode `0600`).
- **OpenAI Codex OAuth** is the ChatGPT subscription path used by the open-source Codex CLI and tools that explicitly integrate with it. Tokens target the Codex/ChatGPT backend, not a classic Platform API key.
- **Anthropic** restricts Free/Pro/Max OAuth for many third-party products. Prefer `claude setup-token` / official Claude Code flows and re-check [Anthropic’s terms](https://www.anthropic.com/legal) and [Claude Code auth docs](https://code.claude.com/docs/en/authentication). Do not resell multi-tenant access to consumer OAuth.
- **xAI** may allowlist which SuperGrok tiers receive OAuth API tokens; if login works but inference returns 403, use an API key path or upgrade the tier.
- Never commit `credentials.json` or paste tokens into tickets/chat.

## 1. Build and log in

```bash
go build -o llm-gateway ./cmd/gateway

./llm-gateway auth login chatgpt    # browser opens auth.openai.com
./llm-gateway auth login claude     # setup-token / paste
./llm-gateway auth login grok       # prefers ~/.grok/auth.json, else device code

./llm-gateway auth status
```

### Alternatives

```bash
# After official Codex CLI login:
codex login
./llm-gateway auth import chatgpt

# After Claude Code /login on Linux (credentials file):
./llm-gateway auth import claude

# After Grok CLI login (recommended if device page shows "Invalid action"):
./llm-gateway auth import grok
# or force browser device flow: ./llm-gateway auth login grok --device

# Headless ChatGPT (print URL only):
./llm-gateway auth login chatgpt --no-browser
```

**Grok “Invalid action”:** xAI’s `accounts.x.ai/oauth2/device?user_code=…` page often fails when the session is wrong. Prefer `auth import grok` from the Grok CLI store (`~/.grok/auth.json`). Device login now opens the **base** URL and prints the code separately.

Store path override:

```bash
export INJA_GATEWAY_AUTH_FILE=$HOME/.config/inja-gateway/credentials.json
```

## 2. Run the gateway (HTTPS background — recommended)

Claude Code needs a live API. Helpers start **HTTPS on 127.0.0.1:8787** in the background:

```bash
source examples/shell/claude-code-helpers.sh
export KEY=local-dev
cc-gateway-up          # certs + nohup + healthz
cc-gateway-logs        # tail gateway.log (−f / −n N)
# ANTHROPIC_BASE_URL=https://127.0.0.1:8787
cc-gpt                 # or cc-grok / cc-gpt-grok / cc-multi
```

Manual:

```bash
./examples/scripts/gen-localhost-tls.sh
export GATEWAY_TLS_CERT=$PWD/examples/certs/localhost.pem
export GATEWAY_TLS_KEY=$PWD/examples/certs/localhost-key.pem
./llm-gateway -config examples/configs/claude-code-subscriptions.yaml
```

That config sets each provider to:

```yaml
auth: oauth2
oauth:
  credentials: chatgpt   # or claude | grok
```

The process loads tokens from the auth store and **refreshes** ChatGPT/Grok access tokens before expiry (Claude setup-token is long-lived; re-run login when it expires).

## 3. Claude Code (any provider combination)

```bash
# Client only needs a non-empty key when edge_auth is off (server holds OAuth).
KEY=local-dev ./examples/claude-code-multi.sh multi       # Claude + GPT + Grok
KEY=local-dev ./examples/claude-code-multi.sh gpt         # GPT only
KEY=local-dev ./examples/claude-code-multi.sh grok        # Grok only (4.5 + composer-2.5)
KEY=local-dev ./examples/claude-code-multi.sh gpt+grok    # GPT + Grok, no Claude
KEY=local-dev ./examples/claude-code-multi.sh claude+gpt
KEY=local-dev ./examples/claude-code-multi.sh list
```

Or shell helpers:

```bash
source examples/shell/claude-code-helpers.sh
export KEY=local-dev GATEWAY=http://localhost:8787

cc-gpt              # GPT only
cc-grok             # Grok 4.5 + composer-2.5
cc-gpt-grok         # both non-Claude
cc-multi            # all three
cc-run gpt+grok     # any combo
cc-list
```

In session:

```text
/model grok-4.5
/model composer-2.5
/model gpt
/model sonnet
```

### Permanent settings

Use [`examples/claude-code-settings.json.example`](https://github.com/inja-online/llm-gateway/blob/master/examples/claude-code-settings.json.example) and point `ANTHROPIC_BASE_URL` at the gateway. Do **not** put subscription OAuth tokens in Claude Code settings if the gateway holds them — set a dummy/edge key only.

## 4. Profiles / combos

Any mix of `claude`, `gpt`, `grok` with separators `+` `,` `-`:

| Profile | opus | sonnet | haiku / small-fast |
|---|---|---|---|
| **gpt** | sol | gpt (terra) | luna |
| **grok** | grok-4.5 | grok-4.5 | composer-2.5 → grok-build-0.1 |
| **gpt+grok** | grok-4.5 | gpt | composer-2.5 |
| **claude** | opus (4.8) | sonnet (5) | haiku (4.5) |
| **claude+gpt** | opus | gpt | luna |
| **claude+grok** | opus | grok-4.5 | composer-2.5 |
| **multi** | opus | gpt | composer-2.5 |

Upstream pins (2026-07): see `examples/configs/claude-code-subscriptions.yaml`.

**Always-fresh list:** `GET /v1/models?live=1` fans out to each provider’s live catalog (plus aliases). Config-only list is aliases. Refresh helper: `./examples/scripts/refresh-model-catalog.sh`. Maintainers: **`AGENTS.md`**.

Overrides: `CC_OPUS_MODEL`, `CC_SONNET_MODEL`, `CC_HAIKU_MODEL`, `CC_MODEL`, `CC_GROK_HEAVY`, `CC_GROK_FAST`, `CC_GPT_HEAVY=sol`.

## 5. How OAuth is applied upstream

| Store provider | Upstream auth | Typical base URL |
|---|---|---|
| `chatgpt` | `Authorization: Bearer` (refreshed) | `https://chatgpt.com/backend-api/codex` |
| `claude` | `Authorization: Bearer` | `https://api.anthropic.com/v1` |
| `grok` | `Authorization: Bearer` (refreshed) | `https://api.x.ai/v1` |

`auth: oauth2` + `oauth.credentials` uses a **TokenSource** so Anthropic gets Bearer (not `x-api-key`). That matches subscription OAuth tokens.

## 6. Troubleshooting

| Symptom | Fix |
|---|---|
| `no credentials for chatgpt` | `llm-gateway auth login chatgpt` |
| ChatGPT works in Codex but 401 via gateway | Re-login; confirm `auth status` has refresh; check base_url |
| Claude 401 after import | On macOS, Keychain isn’t imported — use `auth login claude` / setup-token |
| Grok 403 after successful login | xAI tier gate — try API key provider or check subscription |
| Token refresh fails `invalid_grant` | `auth logout <provider>` then login again |

## Cursor IDE (same gateway)

Cursor uses OpenAI-compatible **Settings → Models** (not `ANTHROPIC_BASE_URL`):

```bash
source examples/shell/claude-code-helpers.sh
source examples/shell/cursor-helpers.sh
export KEY=local-dev
cc-gateway-up
cursor-setup    # OpenAI key + https://127.0.0.1:8787/v1 + model list
```

| Cursor field | Value |
|---|---|
| OpenAI API Key | `local-dev` (edge key) |
| Override OpenAI Base URL | `https://127.0.0.1:8787/v1` |
| Custom models | `gpt`, `grok-4.5`, `composer-2.5`, `sonnet`, … |

Docs: [Any app integrations](https://inja-online.github.io/llm-gateway/guides/app-integrations/) · [Cursor + subscriptions](https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/) · [`examples/cursor/README.md`](https://github.com/inja-online/llm-gateway/blob/master/examples/cursor/README.md) · [`examples/apps/`](https://github.com/inja-online/llm-gateway/tree/master/examples/apps)

## 7. Related

- [`examples/configs/claude-code-subscriptions.yaml`](https://github.com/inja-online/llm-gateway/blob/master/examples/configs/claude-code-subscriptions.yaml)
- [`llm-gateway auth`](https://github.com/inja-online/llm-gateway/blob/master/cmd/gateway/auth_cmd.go) · package `subauth`
- [oauth-token-sources.md](oauth-token-sources.md) · [claude-code-checklist.md](claude-code-checklist.md)
- OpenAI Codex auth: [learn.chatgpt.com/docs/auth](https://learn.chatgpt.com/docs/auth)
- Claude Code auth: [code.claude.com/docs/en/authentication](https://code.claude.com/docs/en/authentication)
