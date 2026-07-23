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
| `GET /v1/models` | Config aliases only (offline, deterministic) |
| `GET /v1/models?live=1` | Aliases **plus** live `GET {provider.base}/models` for openai / openai_compat / anthropic when credentials resolve; failures skipped |
| `anthropic-version` on `GET /v1/models` | Pure Anthropic upstream proxy (existing path) |

Clients (Cursor, SDKs) that need “what exists on my account” should use **`?live=1`**, not the static alias list alone.

### Stable short names

Keep **short** alias keys stable for UX (`sonnet`, `gpt`, `grok`, `composer-2.5`).  
Change only the **right-hand** `provider/model` target when vendors rename.

UI names (e.g. SuperGrok “Composer 2.5”) may differ from API ids (e.g. `grok-build-0.1`) — map UI→API in comments.

## Subscription OAuth / Claude Code / Cursor

- Auth CLI: `llm-gateway auth login|import|status` (`subauth` package).  
- HTTPS local: `examples/scripts/gen-localhost-tls.sh`, helpers `cc-gateway-up`.  
- Claude Code combos: `examples/claude-code-multi.sh`, `examples/shell/claude-code-*.sh`.  
- Cursor: OpenAI base URL `https://127.0.0.1:8787/v1` — see docs site guides.  
- **ToS:** personal accounts only; no multi-tenant resale of consumer OAuth.

## Docs site

Website content lives in `website/src/content/docs/`. Pushes to `master` deploy via `.github/workflows/docs.yml` to GitHub Pages.

When shipping operator features, update **both** in-repo `docs/*` and `website/src/content/docs/*` where user-facing.

## Tests

```bash
go test ./proxy/ ./config/ ./subauth/ ./cmd/gateway/ -count=1
```

If you change live models merging, cover hermetic cases in `proxy/models_live_test.go` (no real network).
