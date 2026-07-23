# Platform API proxies

First-class **upstream-owned** resource routes. The gateway does not store agents, files, evals, or batches — it authenticates, routes, and meters.

## What it is

Thin **family proxies**:

| Family | Resolver | Typical base |
|--------|----------|--------------|
| OpenAI / openai_compat | `?provider=` · `X-Provider` · `defaults.openai_dialect` | `…/v1` |
| Anthropic | same with `anthropic_dialect` | `…/v1` |
| Google | `?provider=` · `defaults.google_dialect` | `…/v1beta` |

Each request:

1. Optional edge auth  
2. Resolve provider  
3. Forward method/path/body (+ auth headers)  
4. Relay status/body  
5. Emit **one** usage event (`estimated=true` for resource APIs without token usage)

## How to use (common)

```bash
# OpenAI-shaped — provider from default or query
curl -sS -X POST "$GW/v1/evals" \
  -H "Authorization: Bearer $EDGE_OR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-eval", ...}'

# Force provider when multiple openai_compat hosts exist
curl -sS "$GW/v1/evals?provider=openai"
```

```bash
# Google-shaped
curl -sS "$GW/v1beta/files?provider=google" \
  -H "x-goog-api-key: $KEY"
```

## Route catalog

### Google (`/v1beta/…`)

| Route | Purpose |
|-------|---------|
| `/v1beta/files`, `/v1beta/files/{id}…` | Files API |
| `/v1beta/interactions`, `…/{id}…` | Interactions API |
| `/v1beta/batches`, `…/{id}…` | Batch jobs |
| `/v1beta/cachedContents*` | Context cache CRUD |
| `/v1beta/tunedModels*` | Tuned models |
| `/v1beta/fileSearchStores*` | File Search stores |
| `POST …/models/{m}:batchGenerateContent` | Batch generate |
| `POST …/models/{m}:asyncBatchEmbedContent(s)` | Async batch embed |

### Anthropic (`/v1/…`)

| Route | Purpose |
|-------|---------|
| `/v1/skills*`, `/v1/tunnels*`, `/v1/memory_stores*` | Skills / MCP tunnels / memory |
| `/v1/agents*`, `/v1/sessions*`, `/v1/environments*` | Managed Agents |
| `/v1/messages/batches*` | Message Batches |

### OpenAI family

| Route | Purpose |
|-------|---------|
| `/v1/realtime/client_secrets`, `/calls`, `/translations` | Realtime extras (HTTP) |
| `/v1/evals*` | Evals |
| `/v1/organization/*`, `/v1/organizations/*` | Admin / org APIs |
| `/v1/responses/compact`, `/v1/responses/{id}/input_items*` | Responses depth |
| `DELETE /v1/models/{id}` | Delete fine-tuned model |
| `/v1/videos` list/delete/remix | Video parity extras |
| `/v1/chat/deferred-completion/{id}` | xAI deferred |
| `/v1/rerank`, `/v1/ocr` | Cohere/Mistral-style compat |
| `/v1/vector_stores*`, `/v1/uploads*`, `/v1/containers*` | Storage platform |
| `/v1/fine_tuning/jobs*`, `/v1/batches*` | Fine-tuning / Batches |

## Configuration tips

```yaml
defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google

providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    api_key_env: OPENAI_API_KEY
  anthropic:
    kind: anthropic
    base_url: "https://api.anthropic.com/v1"
    api_key_env: ANTHROPIC_API_KEY
  google:
    kind: google
    base_url: "https://generativelanguage.googleapis.com/v1beta"
    api_key_env: GEMINI_API_KEY
```

Admin / Evals often need **organization admin keys** — use a dedicated provider entry with `api_key_env: OPENAI_ADMIN_KEY` and `?provider=openai_admin`.

## What is not proxied as state

Conversations API remains **501** (stateless gateway). Assistants/Threads are proxied to OpenAI when configured (upstream-owned).

## Related

- [Realtime WebSocket](realtime-websocket.md)
- [OAuth & token sources](oauth-token-sources.md)
- [README HTTP API](https://github.com/inja-online/llm-gateway/blob/master/README.md#http-api)
