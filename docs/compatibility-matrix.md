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

| Modality | OpenAI routes | Status |
|---|---|---|
| TTS `audio_speech` | planned `/v1/audio/speech` | not shipped |
| STT `audio_transcribe` | planned `/v1/audio/transcriptions` | not shipped |

**Operator note (Groq STT-first):** when STT routes land, configure `groq` with `capabilities.audio_transcribe: true` and call `model: groq/<whisper-model>` while leaving `defaults.openai_dialect` on another chat provider. Multipart body limit: **32 MiB**. See `gateway.example.yaml` comments.

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

- [README](../README.md) · [CONTRIBUTING modality guide](../CONTRIBUTING.md#adding-a-modality)
- [Deprecation / field drops](deprecation-policy.md)
- [Claude Code checklist](claude-code-checklist.md)
