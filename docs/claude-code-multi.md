# Claude Code with subscription OAuth (ChatGPT + Claude + SuperGrok + Gemini)

**Last updated:** 2026-09-04

> **Docs site (recommended):** [Claude Code with ChatGPT, Claude, SuperGrok & Gemini](https://inja-online.github.io/llm-gateway/guides/claude-code-subscriptions/) — full public guide with combos, PATH wrappers, live models, and the Gemini client-secret export.

Use **your own consumer subscriptions** (not API keys) through the gateway:

| Provider | Login command | Subscription |
|---|---|---|
| **ChatGPT** | `llm-gateway auth login chatgpt` | ChatGPT Plus / Pro / Team via **Codex OAuth** (PKCE) |
| **Claude** | `llm-gateway auth login claude` | Claude Pro / Max via **setup-token** / Claude Code login |
| **Grok** | `llm-gateway auth login grok` | **SuperGrok** or **X Premium+** via xAI **device-code OAuth** |
| **Gemini** | `llm-gateway auth login gemini` | **Antigravity / Gemini CLI** Jetski token (`google` / `antigravity` aliases) |

Claude Code still speaks **Anthropic Messages**. The gateway passthroughs Claude and **translates** to ChatGPT/xAI/Google when you pick those model aliases.

```
  Claude Code  ──Anthropic──►  llm-gateway  ──┬── Claude (subscription OAuth bearer)
                                              ├── ChatGPT (Codex OAuth refresh)
                                              ├── xAI Grok (device OAuth refresh)
                                              └── Gemini (Antigravity / Jetski OAuth)
```

### Subscription request + model list behavior

1. **Upstream headers** — Claude OAuth betas / Codex `User-Agent`+`Originator`+`Chatgpt-Account-Id`.
2. **TLS** — Chrome-like ClientHello (utls) toward `api.anthropic.com` and `chatgpt.com`.
3. **Claude OAuth body** — tool-name remap to Claude Code TitleCase; for non-`claude-cli` clients, inject billing/`cch` + Claude Code system shape + `metadata.user_id` (cloak `auto`).
4. **Multi-account** — optional `pool` in the credential store; round-robin + 429 cooldown/retry.
5. **`GET /v1/models`** — credential-gated aliases + catalog (static / remote refresh).
6. **Routing** — missing credentials fail at token resolution with a clear error.

## Security & ToS (read this)

- Log in only with **accounts you own**. Credentials are stored **locally** (`~/.config/inja-gateway/credentials.json`, mode `0600`).
- **OpenAI Codex OAuth** is the ChatGPT subscription path used by the open-source Codex CLI and tools that explicitly integrate with it. Tokens target the Codex/ChatGPT backend, not a classic Platform API key.
- **Anthropic** restricts Free/Pro/Max OAuth for many third-party products. Prefer `claude setup-token` / official Claude Code flows and re-check [Anthropic’s terms](https://www.anthropic.com/legal) and [Claude Code auth docs](https://code.claude.com/docs/en/authentication). Do not resell multi-tenant access to consumer OAuth.
- **xAI** may allowlist which SuperGrok tiers receive OAuth API tokens; if login works but inference returns 403, use an API key path or upgrade the tier.
- Never commit `credentials.json` or paste tokens into tickets/chat.
- **Gemini / Antigravity:** personal accounts only. Token import is not enough to refresh — set `INJA_GATEWAY_GEMINI_CLIENT_SECRET` (never commit `GOCSPX-…`).

## 1. Build and log in

```bash
go build -o llm-gateway ./cmd/gateway

./llm-gateway auth login chatgpt    # browser opens auth.openai.com
./llm-gateway auth login claude     # setup-token / paste
./llm-gateway auth login grok       # prefers ~/.grok/auth.json, else device code
./llm-gateway auth login gemini     # ~/.gemini/jetski-standalone-oauth-token (aliases: google, antigravity)

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

# After Gemini CLI / Antigravity login:
export INJA_GATEWAY_GEMINI_CLIENT_SECRET='GOCSPX-…'   # matching desktop client; never commit
./llm-gateway auth import gemini

# Headless ChatGPT (print URL only):
./llm-gateway auth login chatgpt --no-browser
```

**Grok “Invalid action”:** xAI’s `accounts.x.ai/oauth2/device?user_code=…` page often fails when the session is wrong. Prefer `auth import grok` from the Grok CLI store (`~/.grok/auth.json`). Device login now opens the **base** URL and prints the code separately.

**Gemini client secret:** `~/.gemini/jetski-standalone-oauth-token` has access/refresh tokens, **not** Google’s OAuth client secret. Refresh needs the secret for client id `1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com`. Export it from [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials) (that Desktop client only). Then:

```bash
export INJA_GATEWAY_GEMINI_CLIENT_SECRET='GOCSPX-…'  # never commit
llm-gateway auth import gemini   # copies secret into 0600 store when env is set
```

Do **not** reuse a `gcloud` / Cloud Code ADC `GOCSPX-…` — different client. If you cannot open that OAuth client in Console, use a Gemini API key instead. Full steps: [docs site](https://inja-online.github.io/llm-gateway/guides/claude-code-subscriptions/).

Store path override:

```bash
export INJA_GATEWAY_AUTH_FILE=$HOME/.config/inja-gateway/credentials.json
```

## 2. Install helpers + run the gateway (HTTPS background — recommended)

Shell helpers (`cc-gateway-up`, `cc-gpt`, `cursor-apply`, `apps-use-gateway`, …) ship **inside the binary**:

```bash
llm-gateway helpers install          # or: llm-gateway load-helpers
eval "$(llm-gateway helpers source)" # → ~/.config/inja-gateway/shell/
# git checkout alternative: source examples/shell/claude-code-helpers.sh
```

Claude Code needs a live API. Helpers start **HTTPS on 127.0.0.1:8787** in the background:

```bash
export KEY=local-dev
cc-gateway-up          # certs + nohup + healthz
cc-gateway-logs -f     # http + usage (model, tokens, latency)
# ANTHROPIC_BASE_URL=https://127.0.0.1:8787
cc-gpt                 # or cc-grok / cc-gemini / cc-gpt-grok / cc-multi
```

Manual:

```bash
./examples/scripts/gen-localhost-tls.sh
export GATEWAY_TLS_CERT=$PWD/examples/certs/localhost.pem
export GATEWAY_TLS_KEY=$PWD/examples/certs/localhost-key.pem
./llm-gateway -config examples/configs/claude-code-subscriptions.yaml
# installed config path: ~/.config/inja-gateway/claude-code-subscriptions.yaml
```

That config sets each provider to:

```yaml
auth: oauth2
oauth:
  credentials: chatgpt   # or claude | grok | gemini
```

The process loads tokens from the auth store and **refreshes** ChatGPT/Grok/Gemini access tokens before expiry (Claude setup-token is long-lived; re-run login when it expires). Gemini refresh also needs `INJA_GATEWAY_GEMINI_CLIENT_SECRET`.

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

Or shell helpers (after `helpers install` or `source examples/shell/…`):

```bash
export KEY=local-dev

cc-gpt              # GPT only
cc-grok             # Grok 4.5 + composer-2.5
# PATH shims (after helpers install):
#   ln -sf ~/.config/inja-gateway/scripts/claude-grok ~/.local/bin/claude-grok
#   ln -sf ~/.config/inja-gateway/scripts/claude-gemini ~/.local/bin/claude-gemini
#   ln -sf ~/.config/inja-gateway/scripts/claude-codex ~/.local/bin/claude-codex
#   claude-grok --help   # forwarded to claude; no gateway
#   claude-grok          # Grok 4.6 + xhigh
#   claude-gemini        # live Google model from GET /v1/models?live=1
#   claude-codex         # ChatGPT / Codex
cc-gemini           # Gemini / Antigravity (ccgm)
cc-gpt-grok         # both non-Claude
cc-multi            # Claude + GPT + Grok
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

### Thinking, xhigh, ultracode (custom models)

Claude Code treats unknown `ANTHROPIC_BASE_URL` model ids as having **no** thinking / effort / ultracode. Gateway aliases (`grok-4.6`, `gemini`, `sol`, …) are unknown unless you advertise capabilities.

Helpers (`cc-*`, `claude-grok` / `claude-gemini` / `claude-codex`) now:

1. Export `CLAUDE_CODE_EFFORT_LEVEL=xhigh` and `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU}_MODEL_SUPPORTED_CAPABILITIES` plus `ANTHROPIC_CUSTOM_MODEL_OPTION` = the session model, with `effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking`.
2. Pass `--effort xhigh` unless you already passed `--effort`.
3. Write a **session** `--settings` JSON (`ultracode: true`, `alwaysThinkingEnabled`, `effortLevel: xhigh`, `modelPicker.options` with `behavesAs: "claude-fable-5"` for gateway aliases). Ultracode is **session-scoped** (xhigh + Workflow API); it does **not** persist in `~/.claude/settings.json`. Override path: `CC_ULTRACODE_SETTINGS`. Pass your own `--settings` to skip the helper file.

`CLAUDE_CODE_DISABLE_WORKFLOWS` / `CLAUDE_CODE_DISABLE_THINKING` turn the features off. `behavesAs` is honored from user / `--settings` / managed settings only — not project checkout.

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
| **gemini** | gemini-pro | gemini | gemini-flash |
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
| `gemini` | `Authorization: Bearer` (refreshed; needs client secret) | `https://generativelanguage.googleapis.com/v1beta` |

`auth: oauth2` + `oauth.credentials` uses a **TokenSource** so Anthropic gets Bearer (not `x-api-key`). That matches subscription OAuth tokens.

## 6. Troubleshooting

| Symptom | Fix |
|---|---|
| `no credentials for chatgpt` | `llm-gateway auth login chatgpt` |
| ChatGPT works in Codex but 401 via gateway | Re-login; confirm `auth status` has refresh; check base_url |
| Claude 401 after import | On macOS, Keychain isn’t imported — use `auth login claude` / setup-token |
| Grok 403 after successful login | xAI tier gate — try API key provider or check subscription |
| Token refresh fails `invalid_grant` | `auth logout <provider>` then login again |
| Gemini refresh `invalid_client` | Set `INJA_GATEWAY_GEMINI_CLIENT_SECRET` for the **Jetski** client id (not a gcloud ADC secret). See docs site. |
| `missing …/examples/scripts/gen-localhost-tls.sh` | Re-run `llm-gateway helpers install` (script lives at `~/.config/inja-gateway/scripts/`). Or: `bash $REPO/examples/scripts/gen-localhost-tls.sh ~/.config/inja-gateway/certs` |
| `_inja_cc_normalize_providers:read:16: bad option: -a` | zsh + stale helpers. Re-`source` after `llm-gateway helpers install` (split is now portable). |
| No thinking / no ultracode on grok/gemini/gpt | Unknown custom model. Re-`helpers install` + source; launch via `cc-*` / wrappers (they inject caps + `--settings` ultracode). Do not set `CLAUDE_CODE_DISABLE_WORKFLOWS`. |

## Cursor IDE (same gateway)

Cursor uses OpenAI-compatible **Settings → Models** (not `ANTHROPIC_BASE_URL`):

```bash
# helpers already installed: eval "$(llm-gateway helpers source)"
export KEY=local-dev
cc-gateway-up
cursor-setup    # OpenAI key + https://127.0.0.1:8787/v1 + model list
cursor-apply    # automate custom models (quit Cursor first)
```

| Cursor field | Value |
|---|---|
| OpenAI API Key | `local-dev` (edge key) |
| Override OpenAI Base URL | `https://127.0.0.1:8787/v1` |
| Custom models | `gpt`, `grok-4.5`, `composer-2.5`, `sonnet`, … |

Docs: [Any app integrations](https://inja-online.github.io/llm-gateway/guides/app-integrations/) · [Cursor + subscriptions](https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/) · [`examples/cursor/README.md`](https://github.com/inja-online/llm-gateway/blob/master/examples/cursor/README.md) · [`examples/apps/`](https://github.com/inja-online/llm-gateway/tree/master/examples/apps)

## 7. Related

- [Claude app + subscriptions](claude-desktop-subscriptions.md) · [Codex + subscriptions](codex-subscriptions.md)
- [`examples/configs/claude-code-subscriptions.yaml`](https://github.com/inja-online/llm-gateway/blob/master/examples/configs/claude-code-subscriptions.yaml)
- [`llm-gateway auth`](https://github.com/inja-online/llm-gateway/blob/master/cmd/gateway/auth_cmd.go) · package `subauth`
- [oauth-token-sources.md](oauth-token-sources.md) · [claude-code-checklist.md](claude-code-checklist.md)
- OpenAI Codex auth: [learn.chatgpt.com/docs/auth](https://learn.chatgpt.com/docs/auth)
- Claude Code auth: [code.claude.com/docs/en/authentication](https://code.claude.com/docs/en/authentication)
