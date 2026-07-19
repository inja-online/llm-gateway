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

### Added

- **Experimental Completions / DeepSeek FIM:** `POST /v1/completions` and `POST /beta/completions` OpenAI-family passthrough (model rewrite + usage). `/beta` rewrites provider base `…/v1` or host root → `…/beta` for DeepSeek FIM. Not multi-dialect translated.
- **Docs:** [SDK hermetic compatibility matrix](docs/sdk-compatibility-matrix.md) (OpenAI/Anthropic/Google; named hermetic tests; default CI has no `-tags live`).
- **Docs:** Moonshot/Kimi token-estimate + balance helper routes **skipped** (call regional Moonshot base directly); DeepSeek FIM marked experimental under Provider notes.
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
