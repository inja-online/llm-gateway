# Inja LLM Gateway

**Small, dependency-free LLM API gateway** — OpenAI, Anthropic, and native Gemini dialects, multi-provider routing, and usage hooks. One static binary for laptop, Docker, or Kubernetes.

[![CI](https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](https://github.com/inja-online/llm-gateway/blob/master/LICENSE)

```
  OpenAI SDK / Anthropic SDK / Gemini client / Claude Code / curl
                            │
                            ▼
                    ┌───────────────┐
                    │  llm-gateway  │──► JSONL / webhook / Go hook
                    └───────┬───────┘
                            │
         ┌──────────┬───────┼────────┬────────────┐
         ▼          ▼       ▼        ▼            ▼
      OpenAI   Anthropic  Google  OpenAI-compat  …
```

| | |
|---|---|
| **Stateless** | No DB or sticky sessions — scale identical replicas |
| **Deps** | Runtime: `gopkg.in/yaml.v3` only |
| **Repo** | [github.com/inja-online/llm-gateway](https://github.com/inja-online/llm-gateway) |
| **Module** | [`github.com/inja-online/llm-gateway`](https://pkg.go.dev/github.com/inja-online/llm-gateway) |

## Start here

1. **[Install & quickstart](getting-started.md)** — binary, Docker, first `healthz`
2. **[OAuth & token sources](oauth-token-sources.md)** — keys, OAuth2, multi-tenant Bearer, SA
3. **[Compatibility matrix](compatibility-matrix.md)** — dialect × modality × provider kind
4. **Full API & config** — [GitHub README](https://github.com/inja-online/llm-gateway/blob/master/README.md)

## Feature guides

| Guide | What you’ll learn |
|---|---|
| [OAuth & token sources](oauth-token-sources.md) | Upstream auth modes, YAML, curl, OpenCode/Claude Code |
| [WIF recipes](wif-recipes.md) | `token_file`, OpenAI WIF, GCP, AWS/Azure, GHA OIDC |
| [Vertex dual-path](vertex-dual-path.md) | AI Studio vs Vertex base URLs + IAM |
| [Realtime & Live WebSocket](realtime-websocket.md) | TLS/`wss`, session limits, Live route |
| [Platform API proxies](platform-apis.md) | Files, evals, agents, batches, admin, video extras |
| [Embeddings](embeddings.md) | dimensions, encoding_format, task_type |
| [Tools policy](tools-policy.md) | Function + custom/server tool union |
| [Chat field parity](chat-field-parity.md) | Which fields PT / IR / drop |
| [Error & stop reasons](error-finish-reason-catalog.md) | finish_reason / stop_reason catalog |
| [SSE protocol catalog](sse-protocol-catalog.md) | Chat / Responses / Anthropic / Gemini streams |
| [M6 surface notes](m6-remaining-surface.md) | Regional, media, bridge policy |

## Ops & providers

| Section | Contents |
|---|---|
| [Getting started](getting-started.md) | Install, config sketch, health check |
| [Any app integrations](https://inja-online.github.io/llm-gateway/guides/app-integrations/) | Claude Desktop, Codex/GPT Desktop, Cursor, Continue, Cline, Aider, Windsurf, SDKs |
| [Claude Code + subscriptions](claude-code-multi.md) | ChatGPT / Claude / SuperGrok OAuth, any combo (`gpt`, `grok`, `gpt+grok`, …) |
| [Cursor + subscriptions](https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/) | Same gateway in Cursor (OpenAI base URL override) |
| [Claude Code checklist](claude-code-checklist.md) | Anthropic base URL + release regression |
| [Compatibility](compatibility-matrix.md) | P / T / U matrix |
| [SDK hermetic matrix](sdk-compatibility-matrix.md) | CI anchors |
| [Deprecation policy](deprecation-policy.md) | Translation field drops |
| [Z.AI](providers/zai.md) · [Qwen](providers/qwen.md) · [xAI](providers/xai.md) · [Groq STT](providers/groq-stt.md) · [DeepSeek FIM](providers/deepseek-fim.md) | Regional providers |
| [service_tier / fingerprint](service-tier-fingerprint.md) | OpenAI optional metadata |
| [cache_control policy](cache-control-policy.md) | Anthropic caching |
| [Header matrix](header-matrix.md) | Rate-limit / request-id |
| [Security (multipart)](security-multipart-review.md) | Size limits, SSRF posture |
| [Contributing](contributing.md) | Dev checklist |
| [Changelog](changelog.md) | Release history |
| [License](license.md) | AGPL-3.0 |

!!! tip "Source of truth"
    Route tables and YAML field lists also live in the repository [README](https://github.com/inja-online/llm-gateway/blob/master/README.md). The docs site organizes operator-facing guides for browsing and search.
