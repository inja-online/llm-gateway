# Agent instructions (Inja LLM Gateway)

Read this file at the start of any session that touches **model routing**, **Claude Code / Cursor helpers**, **subscription OAuth**, or **example configs**.

## Model aliases must stay current

Example configs under `examples/configs/` (`claude-code-subscriptions.yaml`, `claude-code-multi.yaml`) define **short aliases** (`sonnet`, `gpt`, `grok-4.5`, …) that map to **full** `provider/upstream-model` ids.

Those upstream ids **change often**. Never invent or leave multi-year-old snapshot ids (e.g. `claude-sonnet-4-20250514`, `gpt-5.1`, `o3` as defaults) without checking vendors **today**.

### When you touch aliases or docs that list models

1. **Verify current vendor ids** (prefer primary docs, date-check):
   - Anthropic: https://platform.claude.com/docs/en/about-claude/models/overview  
   - OpenAI / Codex / ChatGPT: https://openai.com/index/gpt-5-6/ and platform models docs  
   - xAI: https://docs.x.ai/developers/models  
2. **Prefer live discovery** when a gateway is running with credentials:
   ```bash
   curl -sk "$GATEWAY/v1/models"              # config aliases only
   curl -sk "$GATEWAY/v1/models?live=1"       # + live fan-out from providers
   ./examples/scripts/refresh-model-catalog.sh
   ```
3. **Update** `examples/configs/*.yaml` targets, profile defaults in `examples/shell/claude-code-profiles.sh`, Cursor helpers, and website guides in the **same PR**.
4. Stamp a comment in YAML: `# Model aliases (updated YYYY-MM-DD)`.
5. Do **not** commit secrets; certs stay under `examples/certs/` (gitignored).

### Runtime behavior (do not “fix” by hardcoding forever)

| Endpoint | Behavior |
|----------|----------|
| `GET /v1/models` | Config aliases (offline) **filtered** by usable `oauth.credentials` store entries; plus static subscription catalog ids for logged-in chatgpt/claude/grok |
| `GET /v1/models?live=1` | Above **plus** live `GET {provider.base}/models` for openai / openai_compat / anthropic when credentials resolve; failures skipped |
| `anthropic-version` on `GET /v1/models` | Pure Anthropic upstream proxy (existing path) |

Subscription OAuth (`oauth.credentials`) also injects CLI-compatible upstream headers (Claude OAuth betas, Codex UA / `Chatgpt-Account-Id`) — see `docs/oauth-token-sources.md`.

Clients (Cursor, SDKs) that need “what exists on my API-key account” should use **`?live=1`**. Consumer subscription catalogs are local + credential-gated (Codex has no useful public `/models`).

### Stable short names

Keep **short** alias keys stable for UX (`sonnet`, `gpt`, `grok`, `composer-2.5`).  
Change only the **right-hand** `provider/model` target when vendors rename.

UI names (e.g. SuperGrok “Composer 2.5”) may differ from API ids (e.g. `grok-build-0.1`) — map UI→API in comments.

### Cursor coexistence prefixes

Cursor keeps built-ins (e.g. **Claude Fable 5**, Composer 2.5). Custom OpenAI models should use **prefixed** aliases so both appear in the picker:

- `claude/fable-5`, `claude/sonnet-5` → Anthropic via gateway  
- `chatgpt/terra`, `chatgpt/sol` → ChatGPT sub via gateway  
- `grok/4.5`, `grok/composer-2.5` → SuperGrok via gateway  
- `inja/…` → same targets, explicit gateway tag  

Helpers: `cursor-models`, `examples/cursor/models-to-add.txt`. Short aliases stay for Claude Code; prefixes are for Cursor dual-list.

## Subscription OAuth / Claude Code / Cursor

- Auth CLI: `llm-gateway auth login|import|status` (`subauth` package).  
- **Embedded helpers:** `llm-gateway helpers install` / `load-helpers` → `~/.config/inja-gateway/shell/` (source of truth for release binaries). Keep `cmd/gateway/shell/*.sh` and `examples/shell/*.sh` in sync (CI test).  
- HTTPS local: `examples/scripts/gen-localhost-tls.sh`, helpers `cc-gateway-up`, logs `cc-gateway-logs`.  
- Claude Code combos: `examples/claude-code-multi.sh`, `cc-gpt` / `cc-grok` / `cc-multi`.  
- Cursor: OpenAI base `…/v1` + **prefixed** custom models (`cursor-apply`) next to Cursor built-ins.  
- **ToS:** personal accounts only; no multi-tenant resale of consumer OAuth.

## Docs site

Website content lives in `website/src/content/docs/`. Pushes to `master` deploy via `.github/workflows/docs.yml` to GitHub Pages.

When shipping operator features, update **both** in-repo `docs/*` and `website/src/content/docs/*` where user-facing.

## Tests

```bash
go test ./proxy/ ./config/ ./subauth/ ./cmd/gateway/ -count=1
```

If you change live models merging, cover hermetic cases in `proxy/models_live_test.go` (no real network).
