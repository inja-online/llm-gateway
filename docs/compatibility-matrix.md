# Compatibility matrix

**Last updated:** 2026-07-19  
**Owning tests:** `proxy/*_test.go`, `config/capability_test.go` (hermetic; no live vendor calls in CI)

This matrix reflects **implemented** gateway behavior for dialect × modality × provider kind. It is not a live vendor certification.

## Legend

| Symbol | Meaning |
|---|---|
| **P** | Native **passthrough** (same family wire; model rewrite + auth + metering) |
| **T** | **Translated** via canonical IR |
| **U** | **Unsupported** (fail closed with dialect-shaped 4xx; no upstream call when capability off) |
| **—** | Not applicable / no ingress for that dialect×modality yet |
| **opt** | `openai_compat` requires explicit `capabilities.<modality>: true` |

## Text chat

| Ingress dialect | Route | `openai` | `openai_compat` | `anthropic` | `google` |
|---|---|---|---|---|---|
| OpenAI | `POST /v1/chat/completions` | **P** | **P** | **T** | **T** |
| Anthropic | `POST /v1/messages` | **T** | **T** | **P** | **T** |
| Google | `POST /v1beta/models/{m}:generateContent` (+ stream) | **T** | **T** | **T** | **P** |

Also: `POST /v1/messages/count_tokens` (Anthropic proxy or estimate).

## Image generation

| Ingress | Route | `openai` | `openai_compat` | `anthropic` | `google` |
|---|---|---|---|---|---|
| OpenAI | `POST /v1/images/generations` (+ edits/variations) | **P** | **P** (opt) | **U** | **U**\* |
| Anthropic-gateway | `POST /v1/images` (planned Media Contract v1) | — | — | — | — |
| Google | `:generateImages` / Imagen (planned) | — | — | — | — |

\*Native Gemini **image-in-chat** uses generateContent multimodal, not `/v1/images/*`.  
OpenAI-compat hosts (OpenRouter, xAI Imagine, etc.) need `capabilities.image_gen: true`.

## Video generation

| Ingress | Route | `openai` | `openai_compat` | `anthropic` | `google` |
|---|---|---|---|---|---|
| OpenAI | `POST /v1/videos`, `GET /v1/videos/{id}` | **P** | **P** (opt) | **U** | **U** |
| Google LRO | planned | — | — | — | — |

## Audio (HTTP)

| Ingress | Route | `openai` | `openai_compat` | `anthropic` | `google` |
|---|---|---|---|---|---|
| OpenAI TTS | `POST /v1/audio/speech` | **P** | **P** (opt) | **U** | **T** (→ generateContent AUDIO) |
| OpenAI STT | `POST /v1/audio/transcriptions`, `/translations` | **P** | **P** (opt) | **U** | **U** |
| Anthropic-gateway TTS | `POST /v1/audio/speech` + `anthropic-version` | **T** | **T** (opt) | **U** | **T** |
| Anthropic-gateway STT | `POST /v1/audio/transcriptions` (+ translations) + `anthropic-version` | **T** | **T** (opt) | **U** | **U** |
| Google TTS | `POST /v1beta/models/{m}:generateSpeech` | **T** (wrap binary) | **T** (opt) | **U** | **T** (→ generateContent AUDIO) |

**Fidelity:** same-family TTS body + multipart STT are byte-passthrough; translation uses base64 wrap/unwrap only (no codec re-encode). Multipart limit **32 MiB**.

**Operator note (Groq STT-first):** configure `groq` with `capabilities.audio_transcribe: true` and call `model: groq/<whisper-model>` (or alias `whisper-fast`) while leaving `defaults.openai_dialect` on another chat provider. Full guide: [providers/groq-stt.md](providers/groq-stt.md).

## Prompt caching (#108)

| Ingress → Egress | Behavior |
|---|---|
| Anthropic → Anthropic | **P** / **T** preserve `cache_control` breakpoints |
| OpenAI → OpenAI | **P** (body) / **T** preserve `prompt_cache_key` / `prompt_cache_retention` |
| Google → Google | **P** / **T** preserve `cachedContent` resource name |
| Cross-family | **Drop** foreign cache directives (no illegal wire fields) |

Usage: cache read/write tokens map when upstream reports them (all dialects).

## Conversations (OpenAI stateful threads)

| Ingress | Route | All provider kinds |
|---|---|---|
| OpenAI | `/v1/conversations*` (nested included) | **U** — HTTP **501** `not_implemented` (stateless; no gateway store). Prefer Responses + client state / Files. Decision: [conversations-decision.md](conversations-decision.md) (Option A). |

## Realtime (WebSocket)

| Ingress | `openai` | `openai_compat` | `anthropic` | `google` |
|---|---|---|---|---|
| OpenAI `/v1/realtime` | **P** (passthrough) | **P** (opt-in `realtime`) | **U** (no Anthropic WS) | **U** fail-closed `unsupported_realtime_bridge` |
| Google Live | **U** fail-closed `unsupported_realtime_bridge` | **U** same | **U** | **P** (passthrough) |

**Bridge:** OpenAI Realtime ↔ Google Live IR bridge is **not implemented** (deferred). Clients must use the native protocol URL for each provider family.

## Capability defaults by kind

| Kind | text | image_gen | video_gen | audio_* | realtime |
|---|---|---|---|---|---|
| `openai` | on | on | on | on | on |
| `google` | on | on | on | on | on |
| `anthropic` | on | off | off | off | off |
| `openai_compat` | on | **off** | **off** | **off** | **off** |

## CI enforcement

- Default CI: `go test ./... -race` air-gapped (`httptest` fakes only).
- No `-tags live` in default workflows.
- Capability matrix e2e cells expand as M4 modalities land (`proxy` package).

## SDK hermetic strategy

Official SDK wire coverage (headers, happy paths, no live keys): **[sdk-compatibility-matrix.md](sdk-compatibility-matrix.md)**.

Named hermetic anchors: `proxy.TestNonStreamPassthrough` (OpenAI), `proxy.TestAnthropicStreamPassthrough` (Anthropic), `proxy.TestGooglePassthroughStreamAndErrors` (Google).

## Related

- [README](https://github.com/inja-online/llm-gateway/blob/master/README.md) · [CONTRIBUTING modality guide](https://github.com/inja-online/llm-gateway/blob/master/CONTRIBUTING.md#adding-a-modality)
- [Deprecation / field drops](deprecation-policy.md)
- [Claude Code checklist](claude-code-checklist.md)
