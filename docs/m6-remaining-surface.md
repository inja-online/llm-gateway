# M6 remaining surface (docs lock-in)

Companion to shipped proxy/fidelity waves. Documents behavior for issues that are **passthrough-complete** or **policy-complete** without inventing incomplete bridges.

## Regional openai_compat (#116, #140, #141, #144)

| Provider | Kind | Notes |
|----------|------|-------|
| xAI | openai_compat | Chat/Responses; deferred via `/v1/chat/deferred-completion/{id}`; Imagine needs `image_gen` |
| Moonshot/Kimi | openai_compat | Helpers: estimate-token-count, balance; regional base_url |
| Z.AI/GLM | openai_compat | Regional bases — [providers/zai.md](providers/zai.md); native extras use same base when vendor exposes OpenAI-shaped paths |
| DeepSeek | openai_compat | Completions/FIM; Anthropic-compat hosts use `kind: anthropic` + DeepSeek base if vendor supports |
| Groq | openai_compat | STT split — [providers/groq-stt.md](providers/groq-stt.md); Responses when base supports |

**Fidelity rule:** passthrough never strips unknown keys. Regional quirks are base_url + capability flags, not custom dialects.

## Google gaps (#117, #159)

| Gap | Status |
|-----|--------|
| Interactions | `/v1beta/interactions*` proxy (#134) |
| finishReason | catalog in [error-finish-reason-catalog.md](error-finish-reason-catalog.md) |
| STT → Google | use OpenAI-compat audio path or Google speech generate; reverse STT not inventing a fake Google STT dialect |
| reverse embeddings (Google→OpenAI native) | Clients should call `POST /v1/embeddings`; native google embed translate is OpenAI→Google |
| speechConfig multi-speaker | passthrough on generateContent body; no translate invent |
| responseModalities IMAGE/AUDIO | passthrough on google family |

## Completions multi-dialect (#125)

| Ingress | Behavior |
|---------|----------|
| OpenAI `/v1/completions` | openai/openai_compat passthrough (incl. DeepSeek FIM on `/beta`) |
| Translate to Anthropic/Google | **Not supported** — fail closed (legacy Completions is OpenAI-shaped only) |

## Anthropic beta / content blocks (#111, #131)

- `anthropic-beta` header: **forwarded** (not allowlisted) on passthrough
- Skills, tunnels, memory stores, agents/sessions/environments: **proxied**
- Unknown content blocks on passthrough: **forwarded**; on translate: only IR block types rebuilt

## Media / audio / files (#145–#149, #155)

| Surface | Behavior |
|---------|----------|
| Image streaming partials | Passthrough SSE/body when upstream streams; translate path non-stream |
| STT diarization / formats | Multipart fields forwarded on openai family |
| Moderations multimodal | `/v1/moderations` passthrough (body including images when OpenAI accepts) |
| Files purposes / large upload | `max_body_bytes` configurable (default 32MiB; set `536870912` for 512MiB); Uploads API for resumable |

```yaml
max_body_bytes: 536870912  # 512 MiB
```

## Citations / grounding (#160)

Canonical `BlockCitation` + `GroundingMetadata` raw JSON. Cross-family invent of citations is **not** performed; Google grounding metadata preserved on google IR when present as raw.

## Realtime ↔ Live bridge (#62, #114)

**Shipped decision:** fail-closed with `unsupported_realtime_bridge`. Same-protocol TLS WS works. IR types in `canonical/realtime.go` reserve layout; full event mappers are a future milestone, not a silent half-bridge.

## Program epic (#1)

M6 surface tracked by this doc + closed issues on the milestone board.
