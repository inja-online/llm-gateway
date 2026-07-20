# Changelog

All notable changes to **Inja LLM Gateway** (`llm-gateway`) are documented here.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/) for the **public HTTP/WS surface and config schema** (not only the Go module path).

## Versioning policy

| Bump | When |
|---|---|
| **MAJOR** | Breaking wire or config changes: removed routes, renames that break clients, required new headers, or drop-list behavior that previously preserved fields |
| **MINOR** | Additive routes, optional config, new provider templates, capability flags, docs |
| **PATCH** | Bug fixes, security hardening, performance, test/CI-only |

**Gateway Media Contract v1** (Anthropic/Google-shaped media paths, when shipped) is versioned with the gateway: additive media fields → MINOR; breaking media field renames → MAJOR. See [docs/deprecation-policy.md](docs/deprecation-policy.md) for translation field drops.

Release process: tag `vX.Y.Z` → GitHub Actions builds multi-arch binaries. PRs that change the public surface should add a changelog entry under `[Unreleased]`.

## [Unreleased]

### Changed

- **License:** project relicensed from MIT to **[GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE)**. Network use of modified versions requires offering corresponding source under the AGPL.
- **Docs cleanup:** removed internal Superpowers SDD/plan trees (`.superpowers/`, `docs/superpowers/`); public contract stays in README + `docs/*.md`.
- **Product policy:** no more **wontfix / document-skip** for missing endpoints. Incomplete items reopened on GitHub; full surface tracked under [milestone M6](https://github.com/inja-online/llm-gateway/milestone/7) (#104–#164 + reopened stubs). README no longer marks Prometheus/health/Moonshot helpers as permanent wontfix.

### Added

- **Fidelity:** OpenAI `service_tier` request + `system_fingerprint` / response `service_tier` passthrough; never invent on Anthropic/Google translate ([#51](https://github.com/inja-online/llm-gateway/issues/51)); [docs/service-tier-fingerprint.md](docs/service-tier-fingerprint.md).
- **Policy:** Non-function OpenAI tools **error** on translation path; passthrough still forwards wire tools ([#49](https://github.com/inja-online/llm-gateway/issues/49)); [docs/tools-policy.md](docs/tools-policy.md).
- **Policy:** Anthropic `cache_control` **passthrough-only** (Option B); translate strips breakpoints ([#41](https://github.com/inja-online/llm-gateway/issues/41)); [docs/cache-control-policy.md](docs/cache-control-policy.md).
- **Config:** optional `observe_dropped_fields` → response `X-Gateway-Dropped-Fields` + usage `dropped_fields` (names only) on translate ([#152](https://github.com/inja-online/llm-gateway/issues/152)).
- **Moonshot helpers:** `POST /v1/tokenizers/estimate-token-count` and `GET /v1/users/me/balance` thin openai_compat proxy ([#89](https://github.com/inja-online/llm-gateway/issues/89), [#137](https://github.com/inja-online/llm-gateway/issues/137)).
- **Docs:** DeepSeek experimental Completions/FIM operator guide ([#90](https://github.com/inja-online/llm-gateway/issues/90)); [docs/providers/deepseek-fim.md](docs/providers/deepseek-fim.md).
- **OpenAI Batches API** proxy: `POST/GET /v1/batches`, `GET …/{id}`, `POST …/{id}/cancel` for openai/openai_compat ([#109](https://github.com/inja-online/llm-gateway/issues/109)).
- **Ops:** optional `GET /v1/health/providers` when `health_checks.enabled` (timeouts, no key logging) ([#94](https://github.com/inja-online/llm-gateway/issues/94), [#153](https://github.com/inja-online/llm-gateway/issues/153)).
- **Ops:** `GET /metrics` Prometheus text counters (requests/tokens; no external deps) ([#95](https://github.com/inja-online/llm-gateway/issues/95), [#154](https://github.com/inja-online/llm-gateway/issues/154)).
- **Models:** live Anthropic `GET /v1/models` (+ `/{id}`) when `anthropic-version` or `?live=1` ([#126](https://github.com/inja-online/llm-gateway/issues/126)).
- **Docs:** [Z.AI / Zhipu regional bases](docs/providers/zai.md) — intl vs CN `openai_compat` examples, date-stamped vendor links, curl sample ([#87](https://github.com/inja-online/llm-gateway/issues/87)).
- **Docs:** [Qwen / DashScope regional bases](docs/providers/qwen.md) — CN vs intl `compatible-mode` URLs, alias samples, README pointer ([#88](https://github.com/inja-online/llm-gateway/issues/88)).
- **Docs:** [xAI Grok / Responses / Imagine](docs/providers/xai.md) — base_url, capabilities, curl + SDK samples ([#91](https://github.com/inja-online/llm-gateway/issues/91)).
- **Docs:** [Groq STT-first routing](docs/providers/groq-stt.md) — split chat/STT YAML, `audio_transcribe`, client curl ([#92](https://github.com/inja-online/llm-gateway/issues/92)).
- **Docs/API:** Conversations documented as **not supported** (501 stub); matrix row + stronger hermetic message tests ([#67](https://github.com/inja-online/llm-gateway/issues/67)).
- **Decision:** Conversations **Option A** (permanent 501; no gateway store; no pure upstream proxy) — [docs/conversations-decision.md](docs/conversations-decision.md) ([#118](https://github.com/inja-online/llm-gateway/issues/118)).
- **Docs:** Deprecation / field-drop policy acceptance locked (#103) — passthrough never drops; no `Warning` header; `x-gateway-dropped-fields` deferred to [#152](https://github.com/inja-online/llm-gateway/issues/152); hermetic doc + drop-list tests.
- **HTTP voice (TTS/STT, M4):** OpenAI `/v1/audio/speech|transcriptions|translations` (passthrough + `kind:google` TTS translation); Anthropic-gateway same paths with `anthropic-version` (translate to OpenAI/Google); Google `POST /v1beta/models/{m}:generateSpeech` → Gemini `generateContent` AUDIO. Capability fail-closed; binary/multipart fidelity tests; usage `audio_speech` / `audio_transcribe`.
- **`GET /v1/models` capability flags:** each catalog entry includes `capabilities` (`chat`, `image_gen`, `video_gen`, `audio_speech`, `audio_transcribe`, `realtime`) from provider kind defaults + YAML overrides (no upstream network).
- **Configurable `max_body_bytes`** (default 32 MiB): oversize requests return HTTP **413** dialect-shaped errors; README limits table expanded (body, header wait, realtime, drain).
- **Multipart/media security review:** [docs/security-multipart-review.md](docs/security-multipart-review.md) linked from SECURITY.md (size limits, filenames, SSRF URI pass-through, `key_hash` only).
- Ops: Prometheus `/metrics` and provider health shipped later under Unreleased (see Added); earlier note deferred to hooks-only.
- **Experimental Completions / DeepSeek FIM:** `POST /v1/completions` and `POST /beta/completions` OpenAI-family passthrough (model rewrite + usage). `/beta` rewrites provider base `…/v1` or host root → `…/beta` for DeepSeek FIM. Not multi-dialect translated.
- **Docs:** [SDK hermetic compatibility matrix](docs/sdk-compatibility-matrix.md) (OpenAI/Anthropic/Google; named hermetic tests; default CI has no `-tags live`).
- **Docs:** Moonshot/Kimi token-estimate + balance helpers tracked open ([#89](https://github.com/inja-online/llm-gateway/issues/89)); DeepSeek FIM experimental under Provider notes ([#90](https://github.com/inja-online/llm-gateway/issues/90)).
- **Anthropic Message Batches** proxy: `POST/GET /v1/messages/batches`, `GET …/{id}`, `POST …/{id}/cancel`, `GET …/{id}/results`. Nested `requests[].params.model` rewrite (aliases / `provider/model`); provider via `?provider=` / `X-Provider` / `defaults.anthropic_dialect` (`kind: anthropic` only). Batches/results are upstream-owned (no gateway storage).
- **Optional edge auth** (`edge_auth`): when `enabled`, require `Authorization: Bearer` or `x-api-key` matching configured keys / `keys_env`. `GET /healthz` stays open. Default **off**.
- **Provider auth modes** for Vertex-style Google hosts: `auth: api_key|adc|service_account|bearer` plus `TokenSource` interface (`StaticTokenSource`, `CachingTokenSource`) and `Server.SetTokenSource` for air-gapped ADC tests (no Google SDK required).
- Forward selected client headers on upstream requests: `HTTP-Referer`, `Referer`, `X-Title`, `OpenAI-Organization`, `OpenAI-Project`, `anthropic-beta`, `anthropic-version` (when set by client).
- Docs: provider notes (OpenRouter, xAI, Z.AI regions, Qwen regions, Groq STT routing), [compatibility matrix](docs/compatibility-matrix.md), [deprecation policy](docs/deprecation-policy.md), [Claude Code checklist](docs/claude-code-checklist.md), CONTRIBUTING modality guide, this changelog.
- **Conversations API stubs** (`/v1/conversations`, `/{id}`, nested paths): HTTP **501** OpenAI envelope `not_implemented` pointing to Responses + client-side state / Files (stateless gateway decision).
- **Realtime bridge fail-closed:** cross-protocol Realtime↔Live attempts return `unsupported_realtime_bridge`; `canonical/realtime.go` placeholder IR reserved for a future milestone.

### Changed

- README Auth & keys section documents edge auth vs upstream `api_key_env`.
- `gateway.example.yaml` expands regional provider examples and edge_auth / Vertex comments.
- README / compatibility matrix: Realtime ↔ Live **bridge deferred** (same-protocol passthrough only); Conversations decision documented as stub 501.

## [0.1.0] — prior

Initial public surface (chat OpenAI/Anthropic/Google, image/video OpenAI-compat passthrough, hooks, Docker/K8s). See git history for pre-changelog releases.
