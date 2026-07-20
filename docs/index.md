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
2. **[Compatibility matrix](compatibility-matrix.md)** — dialect × modality × provider kind
3. **Full API & config** — see the [GitHub README](https://github.com/inja-online/llm-gateway/blob/master/README.md) (HTTP tables, YAML reference, auth, hooks)

## What it does

Clients speak **OpenAI**, **Anthropic**, or **native Gemini**. The gateway:

- **Passthroughs** same-family traffic (near-verbatim fidelity)
- **Translates** cross-dialect chat when client and upstream disagree
- Emits **exactly one usage event** per proxied request (JSONL / webhook / Go hook)

## Docs in this site

| Section | Contents |
|---|---|
| [Getting started](getting-started.md) | Install, config sketch, health check |
| [Claude Code](claude-code-checklist.md) | Anthropic base URL + regression checklist |
| [Compatibility](compatibility-matrix.md) | Implemented P / T / U matrix |
| [SDK hermetic matrix](sdk-compatibility-matrix.md) | CI test anchors, no live vendors |
| [Deprecation policy](deprecation-policy.md) | Translation field drops |
| [Z.AI / Zhipu regions](providers/zai.md) | Intl vs CN `openai_compat` bases |
| [Qwen / DashScope regions](providers/qwen.md) | CN vs intl + `compatible-mode` + aliases |
| [xAI Grok / Imagine](providers/xai.md) | Chat, Responses, image capabilities, samples |
| [Groq STT-first](providers/groq-stt.md) | Split chat + STT providers, curl samples |
| [Security (multipart)](security-multipart-review.md) | Size limits, SSRF posture, logging |
| [Contributing](contributing.md) | Modality checklist |
| [Changelog](changelog.md) | Release history |
| [License](license.md) | AGPL-3.0 summary |

!!! tip "Source of truth"
    Deep route tables and YAML field lists live in the repository [README](https://github.com/inja-online/llm-gateway/blob/master/README.md). This site organizes operator-facing guides and matrices for browsing and search.
