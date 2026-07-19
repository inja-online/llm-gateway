# SDK compatibility matrix (hermetic)

**Last updated:** 2026-07-19  
**Related:** [compatibility-matrix.md](compatibility-matrix.md) (modality × provider kind) · [claude-code-checklist.md](claude-code-checklist.md)

This document is the **hermetic** strategy for official OpenAI / Anthropic / Google client SDK shapes against the gateway. It is **not** a live vendor certification matrix.

## Strategy

| Approach | Use when |
|---|---|
| **Go `net/http` + `httptest` fakes** (default CI) | Assert wire contracts, headers, model rewrite, SSE framing, usage events — no network, no SDK install |
| **Ingress/egress package unit tests** | Dialect JSON parse/build fidelity (OpenAI / Anthropic / Google wire types) |
| **Optional real SDKs** | Manual or opt-in jobs only; still point at a fake upstream or local gateway — **never** require live keys in default CI |

**Default CI** (see [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)):

```text
go test -race -count=1 ./...
```

- No `-tags live`
- No outbound calls to public vendor APIs
- Capability deny cells must not dial upstream (fakes `t.Fatal` on unexpected hits where asserted)

Full SDK installs (Python/JS/Go official clients) are **out of default CI** due to weight and version churn. Hermetic Go tests mimic the critical headers and request shapes those SDKs emit.

## Dialect × hermetic coverage

| Dialect | Critical happy path (Go hermetic) | Package / tests |
|---|---|---|
| **OpenAI** | Chat Completions non-stream + stream passthrough, model rewrite, usage event | `proxy.TestNonStreamPassthrough`, `proxy.TestStreamPassthrough` |
| **Anthropic** | Messages stream passthrough + auth header forward | `proxy.TestAnthropicStreamPassthrough`, `proxy.TestXAPIKeyForwardedToAnthropic` |
| **Google** | Native generateContent / stream passthrough + errors | `proxy.TestGooglePassthroughStreamAndErrors` |

### Supporting hermetic paths (by dialect)

| Dialect | Named tests (representative) |
|---|---|
| OpenAI family | `TestEmbeddingsPassthroughOpenAI`, `TestModerationsPassthrough`, `TestCompletionsV1Passthrough`, `TestCompletionsBetaRewritesBaseToBeta`, `TestOpenRouterPassthroughHeadersAndExtraFields`, `TestOpenAIOrgProjectHeadersForwarded` |
| Anthropic | `TestCountTokensProxiesToAnthropicUpstream`, batches/files tests in `proxy/batches_test.go` / `proxy/*files*` |
| Google | `TestGoogleNativeEmbedContent`, `TestEmbeddingsOpenAIToGoogleSingle`, cross-dialect streams in `proxy/google_extra_test.go` |
| Translate (OpenAI↔Anthropic) | `TestOpenAIToAnthropicNonStream`, `TestOpenAIToAnthropicStream`, fixtures in `proxy/translate_fixtures_test.go` |

Ingress/egress dialect fidelity (not full e2e, still hermetic):

| Dialect | Packages |
|---|---|
| OpenAI | `ingress/openai`, `egress/openai` (`*_test.go`, `fidelity_test.go`) |
| Anthropic | `ingress/anthropic`, `egress/anthropic` |
| Google | `ingress/google`, `egress/google` |

## SDK × route expectations

What a typical **official SDK** needs when `base_url` / host points at the gateway:

| SDK family | Primary routes | Headers / quirks covered hermetically |
|---|---|---|
| OpenAI (Python/JS/Go) | `POST /v1/chat/completions`, `GET /v1/models`, embeddings, responses, files, moderations | Bearer auth; `OpenAI-Organization` / `OpenAI-Project` / `OpenAI-Beta` forward on openai-family egress; stream `include_usage` injection |
| Anthropic | `POST /v1/messages`, `POST /v1/messages/count_tokens` | `x-api-key`, `anthropic-version` default/forward, `anthropic-beta` forward (unknown values allowed) |
| Google GenAI | `POST /v1beta/models/{m}:generateContent`, stream, countTokens, embed | `x-goog-api-key`; path action parse; `?alt=sse` on stream |

**Experimental (not multi-dialect):** DeepSeek FIM via `POST /v1/completions` or `POST /beta/completions` — OpenAI-family passthrough only (`TestCompletions*`).

## What we do *not* claim

- Every SDK minor release automatically certified
- Live key smoke against OpenAI / Anthropic / Google in CI
- Mobile SDKs
- Perfect parity of vendor-only beta features without an explicit gateway route

## Adding coverage

1. Prefer a **hermetic** `proxy` or `ingress`/`egress` test that mimics the SDK request (headers + body).
2. Name the test so it can be linked from this matrix.
3. If introducing a real SDK subprocess job, keep it **opt-in** (separate workflow / `make` target) and still air-gapped with fakes.
4. Update this file and [compatibility-matrix.md](compatibility-matrix.md) “Related” links.

## Related

- End-to-end modality matrix: [compatibility-matrix.md](compatibility-matrix.md)
- Claude Code / Anthropic checklist: [claude-code-checklist.md](claude-code-checklist.md)
- Public API contract: [README.md](../README.md)
