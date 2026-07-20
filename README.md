<p align="center">
  <img src="docs/assets/logo.jpg" alt="Inja LLM Gateway" width="128" height="128" />
</p>

<h1 align="center">Inja LLM Gateway</h1>

<p align="center">
  <strong>Small, dependency-free LLM API gateway</strong><br/>
  OpenAI · Anthropic · Gemini dialects · multi-provider routing · usage hooks<br/>
  One static binary — laptop, Docker, or Kubernetes
</p>

<p align="center">
  <a href="https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml"><img src="https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://github.com/inja-online/llm-gateway/actions/workflows/release.yml"><img src="https://github.com/inja-online/llm-gateway/actions/workflows/release.yml/badge.svg" alt="Release" /></a>
  <a href="https://github.com/inja-online/llm-gateway/releases"><img src="https://img.shields.io/github/v/release/inja-online/llm-gateway?include_prereleases&sort=semver&display_name=tag&label=release&color=blue" alt="Latest release" /></a>
  <a href="https://pkg.go.dev/github.com/inja-online/llm-gateway"><img src="https://pkg.go.dev/badge/github.com/inja-online/llm-gateway.svg" alt="Go Reference" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="AGPL-3.0 License" /></a>
  <a href="https://inja-online.github.io/llm-gateway/"><img src="https://img.shields.io/badge/docs-GitHub%20Pages-blue" alt="Docs" /></a>
  <br/>
  <a href="https://go.dev/dl/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go 1.25+" /></a>
  <a href=".github/workflows/ci.yml"><img src="https://img.shields.io/badge/coverage-%E2%89%A590%25-brightgreen" alt="Coverage ≥90%" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/deps-yaml.v3%20only-informational" alt="yaml.v3 only" /></a>
  <a href="Dockerfile"><img src="https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white" alt="Docker ready" /></a>
  <a href="deploy/k8s/gateway.yaml"><img src="https://img.shields.io/badge/Kubernetes-ready-326CE5?logo=kubernetes&logoColor=white" alt="Kubernetes ready" /></a>
</p>

<p align="center">
  <a href="https://inja-online.github.io/llm-gateway/"><strong>Documentation</strong></a> ·
  <a href="#quickstart">Quickstart</a> ·
  <a href="#http-api">HTTP API</a> ·
  <a href="#configuration">Config</a> ·
  <a href="#deploy">Deploy</a> ·
  <a href="CONTRIBUTING.md">Contributing</a> ·
  <a href="SECURITY.md">Security</a> ·
  <a href="#license">License</a>
</p>

---

Clients speak **OpenAI**, **Anthropic**, or **native Gemini**. The gateway routes to any configured upstream (OpenAI, Anthropic, Google, DeepSeek, xAI, Moonshot, OpenRouter, vLLM, …), **passthroughs** same-family traffic, **translates** cross-dialect chat when needed, and emits **one usage event per request** (JSONL / webhook / Go hook). Stateless — no database.

```
  OpenAI SDK / Anthropic SDK / Gemini client / Claude Code / curl
                            │
                            ▼
                    ┌───────────────┐
                    │  llm-gateway  │──► usage hooks (JSONL / webhook)
                    └───────┬───────┘
                            │
         ┌──────────┬───────┼────────┬────────────┐
         ▼          ▼       ▼        ▼            ▼
      OpenAI   Anthropic  Google  OpenAI-compat  …
                              native   (xAI, DeepSeek, …)
```

| | |
|---|---|
| **Stateless** | No DB, sessions, or sticky routing — scale identical replicas |
| **Cloud-native** | Distroless Docker, K8s sample, SIGTERM drain, env overrides |
| **Local-first** | Single binary on macOS, Linux, Windows; `docker compose up` |
| **Deps** | Runtime: `gopkg.in/yaml.v3` only |
| **Module** | [`github.com/inja-online/llm-gateway`](https://pkg.go.dev/github.com/inja-online/llm-gateway) |
| **License** | [AGPL-3.0](LICENSE) |

**Docs site:** [inja-online.github.io/llm-gateway](https://inja-online.github.io/llm-gateway/) ·  
**Also in-repo:** [compatibility matrix](docs/compatibility-matrix.md) · [SDK hermetic matrix](docs/sdk-compatibility-matrix.md) · [deprecation policy](docs/deprecation-policy.md) · [Claude Code checklist](docs/claude-code-checklist.md) · [multipart security](docs/security-multipart-review.md) · [CHANGELOG](CHANGELOG.md)

---

## Table of contents

- [Quickstart](#quickstart)
- [Features](#features)
- [HTTP API](#http-api)
- [Model routing](#model-routing)
- [Configuration](#configuration)
- [Auth & keys](#auth--keys)
- [Provider notes](#provider-notes)
- [Passthrough vs translation](#passthrough-vs-translation)
- [Hooks & usage events](#hooks--usage-events)
- [Claude Code](#claude-code)
- [Library use](#library-use)
- [Architecture](#architecture)
- [Deploy](#deploy)
- [Development & CI](#development--ci)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [License](#license)

---

## Quickstart

### 1. Binary

```bash
git clone https://github.com/inja-online/llm-gateway.git
cd llm-gateway
go build -o llm-gateway ./cmd/gateway

cp gateway.example.yaml gateway.yaml
# edit providers / keys / hooks
./llm-gateway -config gateway.yaml
# override: GATEWAY_CONFIG=…  GATEWAY_LISTEN=0.0.0.0:8787
```

```bash
curl -s http://localhost:8787/healthz
# {"status":"ok"}
```

### 2. Docker

```bash
docker compose up --build
# or
docker build -t llm-gateway:local .
docker run --rm -p 8787:8787 \
  -e OPENAI_API_KEY -e ANTHROPIC_API_KEY -e GEMINI_API_KEY \
  -v "$PWD/gateway.yaml:/config/gateway.yaml:ro" \
  llm-gateway:local
```

### 3. Kubernetes

```bash
kubectl apply -f deploy/k8s/gateway.yaml
# point the Deployment image at your registry build
```

### Minimal config

```yaml
listen: ":8787"

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
  deepseek:
    kind: openai_compat
    base_url: "https://api.deepseek.com"

defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google

aliases:
  fast: deepseek/deepseek-chat

hooks:
  jsonl:
    output: stdout
```

Full commented sample: [`gateway.example.yaml`](gateway.example.yaml).

---

## Features

| Area | What you get |
|---|---|
| **Triple ingress** | OpenAI Chat Completions, Anthropic Messages (Claude Code), native Gemini `generateContent` |
| **Multi-provider egress** | `openai`, `openai_compat`, `anthropic`, `google` |
| **Passthrough-first** | Same dialect → near-verbatim bytes |
| **Cross-dialect chat** | OpenAI ↔ Anthropic ↔ Google translation (structured outputs, thinking, tools, …) |
| **Media** | Images, video jobs, TTS/STT (OpenAI paths + Anthropic-version gate + Google speech) |
| **Agent surface** | Responses, Files (OpenAI + Anthropic), Message Batches, Moderations |
| **Realtime** | OpenAI Realtime WS + Google Live passthrough; cross-protocol bridge **not** implemented (fail-closed) |
| **Usage metering** | JSONL, async webhook, or in-process Go hook — one event per proxied request |
| **Ops** | One YAML file, `/healthz`, body limits, optional edge auth, multi-arch releases |

---

## HTTP API

### Chat

| Method | Path | Notes |
|---|---|---|
| `POST` | `/v1/chat/completions` | OpenAI dialect (also Gemini OpenAI-compat clients) |
| `GET` / `POST` / `DELETE` | `/v1/chat/completions` · `/{id}` | Stored completions (`store=true`) openai-family proxy |
| `POST` | `/v1/messages` | Anthropic dialect (`anthropic-version` required by clients) |
| `POST` | `/v1/messages/count_tokens` | Anthropic proxy, Google `:countTokens` map, or local estimate |
| `POST` | `/v1beta/models/{model}:generateContent` | Native Gemini |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | Native Gemini SSE (`?alt=sse` upstream) |
| `POST` | `/v1beta/models/{model}:countTokens` | Native Gemini; **no** usage event |

### Discovery & health

| Method | Path | Notes |
|---|---|---|
| `GET` | `/v1/models` | Config-derived catalog + `capabilities`; live Anthropic when `anthropic-version` or `?live=1` |
| `GET` | `/v1/models/{id}` | Single entry (`provider/model` ids); live Anthropic with same triggers |
| `GET` | `/v1beta/models` | Gemini models list passthrough (`?provider=` or `defaults.google_dialect`) |
| `GET` | `/v1beta/models/{model}` | Gemini model get / Live upgrade when `:bidiGenerateContent` |
| `GET` | `/healthz` | `{"status":"ok"}` — process liveness only |
| `GET` | `/v1/health/providers` | Optional upstream probes when `health_checks.enabled` (default off) |
| `GET` | `/metrics` | Prometheus text counters (low cardinality; always on; open with edge auth) |

### Embeddings, Responses, Files, Batches

| Method | Path | Notes |
|---|---|---|
| `POST` | `/v1/embeddings` | OpenAI-family passthrough; OpenAI→Google `embedContent` / batch map |
| `POST` | `/v1beta/models/{m}:embedContent` | Native Gemini embeddings |
| `POST` | `/v1beta/models/{m}:batchEmbedContents` | Native Gemini batch embeddings |
| `POST` | `/v1/responses` | OpenAI Responses (stream SSE supported) |
| `GET` / `DELETE` | `/v1/responses/{id}` | `?provider=` or default OpenAI dialect |
| `POST` / `GET` / `DELETE` | `/v1/files…` | OpenAI Files **or** Anthropic Files when `anthropic-version` is set |
| `GET` | `/v1/files/{id}/content` | Streamed download |
| `POST` / `GET` | `/v1/messages/batches…` | Anthropic Message Batches (`kind: anthropic` only) |
| `POST` / `GET` | `/v1/batches…` | OpenAI Batches (`openai` / `openai_compat`; cancel via `POST …/cancel`) |
| `POST` / `GET` | `/v1/fine_tuning/jobs…` | OpenAI Fine-tuning jobs, cancel, events, checkpoints |
| `POST` | `/v1/moderations` | OpenAI-family passthrough |
| `POST` | `/v1/tokenizers/estimate-token-count` | Moonshot helper (openai_compat; `?provider=` / default) |
| `GET` | `/v1/users/me/balance` | Moonshot balance helper (openai_compat) |

Files and batches are **upstream-owned** (no gateway disk store). Body cap: `max_body_bytes` (default **32 MiB**).

**Anthropic Files** use the same `/v1/files*` paths as OpenAI; presence of `anthropic-version` selects the Anthropic path. Client `anthropic-beta` (including unknown values) is forwarded.

### Media & audio

| Method | Path | Notes |
|---|---|---|
| `POST` | `/v1/images/generations` · `/edits` · `/variations` | OpenAI-shaped |
| `POST` | `/v1/images` · `/v1/images/edits` | Anthropic-gateway when `anthropic-version` set |
| `POST` | `/v1/videos` · `GET /v1/videos/{id}` · `/content` | Video jobs |
| `POST` | `/v1/audio/speech` | TTS (OpenAI path; Anthropic-version → Anthropic-gateway contract) |
| `POST` | `/v1/audio/transcriptions` · `/translations` | STT multipart/JSON |
| `POST` | `/v1beta/models/{m}:generateSpeech` | Google-shaped TTS → Gemini AUDIO `generateContent` |

`openai_compat` media/realtime defaults **off** — opt in with `capabilities` in YAML.

### Realtime (WebSocket)

| Path | Notes |
|---|---|
| `GET /v1/realtime` | OpenAI Realtime upgrade; requires `capabilities.realtime` |
| `GET /v1beta/models/{m}:bidiGenerateContent` | Google Live; `kind: google` + realtime capability |

Same-protocol passthrough only. Cross-protocol Realtime↔Live attempts return **`unsupported_realtime_bridge`**. Session limits: `realtime.max_sessions` (default 1024), `realtime.max_session_minutes` (default 60).

### Completions (experimental)

| Method | Path | Notes |
|---|---|---|
| `POST` | `/v1/completions` | OpenAI-family Completions passthrough |
| `POST` | `/beta/completions` | Rewrites base `…/v1` → `…/beta` (DeepSeek FIM) |

Not multi-dialect translated. Prefer chat for normal use.

### Conversations (not supported)

`/v1/conversations*` (including nested paths such as `/{id}/items`) returns HTTP **501** with an OpenAI-shaped error:

| Field | Value |
|---|---|
| status | **501** |
| `error.type` / code | `not_implemented` |
| guidance | Prefer **`POST /v1/responses`** with **client-side** conversation/history state; use **Files** for durable assets |

**Decision:** permanent skip of gateway-side conversation storage (stateless). Routes are registered so SDKs get a structured 501 instead of a bare 404. Do **not** add a gateway database or Redis thread store.

Alternatives:

1. `POST /v1/responses` (+ get/delete by id on upstream when supported)
2. Client-owned message history on subsequent chat/Responses calls
3. Files / vector-store workstreams for stored assets (upstream-owned)

Formal product decision (**Option A** permanent 501): [docs/conversations-decision.md](docs/conversations-decision.md) ([#118](https://github.com/inja-online/llm-gateway/issues/118)).

### Dialect pairing (chat)

| Client dialect | Upstream | Path |
|---|---|---|
| OpenAI | `openai` / `openai_compat` | passthrough |
| OpenAI | `anthropic` / `google` | translated |
| Anthropic | `anthropic` | passthrough |
| Anthropic | `openai` / `openai_compat` / `google` | translated |
| Google | `google` | passthrough |
| Google | `openai` / `openai_compat` / `anthropic` | translated |

### Limits & timeouts

| Limit | Default | Config / code |
|---|---|---|
| Request/response body | **32 MiB** | `max_body_bytes` (bytes); oversize → **413** |
| HTTP `ReadHeaderTimeout` | 10s | server |
| Upstream `ResponseHeaderTimeout` | 60s | HTTP client |
| Idle conn | 90s | HTTP client |
| count_tokens upstream | 15s | request context |
| Realtime max sessions | 1024 | `realtime.max_sessions` |
| Realtime max duration | 60 min | `realtime.max_session_minutes` |
| SIGTERM drain | 30s | process |
| Webhook hook timeout | 3s | `hooks.webhook.timeout` |

---

## Model routing

Public `model` resolves in order:

1. **`aliases`** — exact match (`fast` → `deepseek/deepseek-chat`)
2. **`provider/model`** — first segment is a configured provider name
3. **Bare id** — dialect default (`defaults.openai_dialect` / `anthropic_dialect` / `google_dialect`)

Missing default or unknown provider → **404** (dialect error envelope).

`GET /v1/models` is built from config only (no upstream fan-out). Each entry may include:

```json
{
  "id": "fast",
  "object": "model",
  "created": 0,
  "owned_by": "llm-gateway",
  "capabilities": {
    "chat": true,
    "image_gen": false,
    "video_gen": false,
    "audio_speech": false,
    "audio_transcribe": false,
    "realtime": false
  }
}
```

Flags come from provider kind defaults + optional YAML `capabilities` (`text` maps to JSON `chat`).

---

## Configuration

Single YAML file. Unknown fields are rejected.

| Field | Required | Description |
|---|---|---|
| `listen` | no | Bind address; default `:8787` (`GATEWAY_LISTEN`) |
| `providers` | yes | Map of name → provider (≥1) |
| `providers.<n>.kind` | yes | `openai` \| `openai_compat` \| `anthropic` \| `google` |
| `providers.<n>.base_url` | yes | Origin **with version prefix**; trailing `/` trimmed |
| `providers.<n>.api_key_env` | no | Env var; when set & non-empty, **replaces** client key |
| `providers.<n>.capabilities` | no | Override modality flags; nil → kind defaults (`openai_compat` = text only) |
| `providers.<n>.auth` | no | `api_key` (default) \| `adc` \| `service_account` \| `bearer` |
| `defaults.openai_dialect` | no | Bare models on OpenAI ingress |
| `defaults.anthropic_dialect` | no | Bare models on Anthropic ingress |
| `defaults.google_dialect` | no | Bare models on Gemini ingress |
| `aliases` | no | Public id → `provider/upstream-model` |
| `max_body_bytes` | no | Default `33554432` (32 MiB) |
| `observe_dropped_fields` | no | Default `false`. When `true`, translate responses set `X-Gateway-Dropped-Fields` (names only) and usage `dropped_fields` |
| `health_checks.enabled` | no | Default `false`. Enables `GET /v1/health/providers` upstream probes |
| `health_checks.timeout` | no | Per-provider probe timeout (default `2s`) |
| `edge_auth` | no | Optional shared-secret gate (see Auth) |
| `realtime.*` | no | Session caps |
| `hooks.jsonl` / `hooks.webhook` | no | Usage sinks |

### Provider kinds

| Kind | Typical base | Auth |
|---|---|---|
| `openai` | `https://api.openai.com/v1` | `Authorization: Bearer` |
| `openai_compat` | DeepSeek, xAI, Moonshot, OpenRouter, Gemini `…/v1beta/openai`, vLLM | Bearer |
| `anthropic` | `https://api.anthropic.com/v1` | `x-api-key` + `anthropic-version` |
| `google` | `https://generativelanguage.googleapis.com/v1beta` | `x-goog-api-key` (or Bearer via `auth: adc`) |

---

## Auth & keys

### Upstream credentials

The gateway reads a client credential from:

1. `Authorization: Bearer <key>`, or  
2. `x-api-key: <key>`, or  
3. `x-goog-api-key: <key>`

…and forwards it using the provider’s scheme, unless `api_key_env` or a TokenSource (`auth: adc` / `service_account`) supplies a server-held credential.

Usage events include `key_hash` (12 hex chars of SHA-256 of the **upstream** credential) — correlate without storing secrets.

### Optional edge auth

By default the gateway does **not** authenticate callers (trusted network / external auth). To require a shared secret:

```yaml
edge_auth:
  enabled: true
  keys_env: GATEWAY_EDGE_KEYS   # comma-separated
```

When enabled, every route **except** `GET /healthz` requires a matching key. Missing/invalid → **401**. Constant-time compare; keys never logged. With `api_key_env` on providers, clients only need the edge key.

See [SECURITY.md](SECURITY.md).

### Forwarded client headers

When present: `HTTP-Referer`, `Referer`, `X-Title`, `OpenAI-Organization`, `OpenAI-Project`, `anthropic-beta`, client `anthropic-version`.  
**`anthropic-beta` is not allowlisted** — unknown / future beta strings are forwarded unchanged.

---

## Provider notes

Full comments: [`gateway.example.yaml`](gateway.example.yaml). Matrices: [docs/compatibility-matrix.md](docs/compatibility-matrix.md), [docs/sdk-compatibility-matrix.md](docs/sdk-compatibility-matrix.md).

| Provider | Kind | Notes |
|---|---|---|
| **OpenAI** | `openai` | Chat, Responses, Files, Moderations, images, video, audio, Realtime |
| **Anthropic** | `anthropic` | Messages, count_tokens, Files (+ beta), Batches |
| **Google native** | `google` | generateContent, embeddings, models list, Live, speech |
| **Gemini OpenAI-compat** | `openai_compat` | `…/v1beta/openai` base; opt-in media capabilities |
| **DeepSeek** | `openai_compat` | Chat + experimental Completions/FIM (`/v1` or `/beta`) |
| **OpenRouter / xAI / Moonshot / Groq / Qwen / …** | `openai_compat` | Passthrough; set `capabilities` for media/realtime |
| **Z.AI / Zhipu (GLM)** | `openai_compat` | **Regional bases** (intl vs CN) — [docs/providers/zai.md](docs/providers/zai.md); wrong region ⇒ auth fail |
| **Qwen (DashScope)** | `openai_compat` | **Regional bases** + `compatible-mode` path — [docs/providers/qwen.md](docs/providers/qwen.md); aliases `qwen-turbo` / `qwen-plus` |
| **xAI (Grok)** | `openai_compat` | Chat + Responses; Imagine images need `image_gen` — [docs/providers/xai.md](docs/providers/xai.md); alias `grok` |
| **Groq** | `openai_compat` | **STT-first** split routing — [docs/providers/groq-stt.md](docs/providers/groq-stt.md); `audio_transcribe` + alias `whisper-fast` |
| **Moonshot helpers** | `openai_compat` | `POST /v1/tokenizers/estimate-token-count`, `GET /v1/users/me/balance` via `?provider=` / default OpenAI dialect (regional base) |
| **Vertex** | `google` + `auth: adc` | Inject TokenSource in library mode; no Google SDK bundled |

---

## Passthrough vs translation

### Passthrough (same family)

Client dialect matches provider kind → near-verbatim proxy: model rewrite, auth, headers, one usage event. Highest fidelity.

### Translation (cross family)

Client and upstream disagree → parse to **canonical IR**, rebuild wire, stream map. Chat fidelity includes tools, structured outputs, thinking/reasoning, sampling knobs, document/audio blocks where mapped. Some vendor-only fields are still dropped — see [docs/deprecation-policy.md](docs/deprecation-policy.md) (passthrough **never** drops; translation drop lists + semver) and `testdata/fixtures/chat_translate/drops/`. Non-function OpenAI tools: **error** on translate, forward on passthrough — [docs/tools-policy.md](docs/tools-policy.md).

**Not** multi-dialect translated: Completions/FIM, most media jobs (family passthrough + limited speech translate), Files, Batches.

---

## Hooks & usage events

Exactly **one** `UsageEvent` per proxied chat, media, embeddings, audio, responses, files, batches create, or realtime session (including errors). Not emitted for `count_tokens`, Gemini `:countTokens`, models discovery, or `healthz`.

| Sink | Config |
|---|---|
| JSONL | `hooks.jsonl.output`: `stdout` \| `stderr` \| file path |
| Webhook | `hooks.webhook.url` (+ optional `timeout`, default 3s) |
| Go | `gateway.WithHook(...)` in library mode |

### Metrics / Prometheus

`GET /metrics` exposes low-cardinality Prometheus text counters (`llm_gateway_requests_*`, `llm_gateway_tokens_*`) with **no extra dependencies**. Open when edge auth is on (like `/healthz`). Prefer hooks for high-cardinality labels / full billing detail.

### Provider health

`/healthz` is **process liveness** only. Optional `GET /v1/health/providers` probes configured upstreams when `health_checks.enabled: true` (timeouts; no key logging).

### Event shape (JSON)

Typical fields: `request_id`, `time`, `dialect_in`, `provider`, `model`, `upstream_model`, `modality`, `transport`, token counts, optional `media`, `stream`, `status`, `http_status`, `latency_ms`, `key_hash`. See [hooks package docs](https://pkg.go.dev/github.com/inja-online/llm-gateway/hooks).

---

## Claude Code

```bash
export ANTHROPIC_BASE_URL=http://localhost:8787
export ANTHROPIC_API_KEY=sk-…   # or edge key when edge_auth is on
# optional: ANTHROPIC_MODEL=deepseek/deepseek-chat
claude
```

Checklist: [docs/claude-code-checklist.md](docs/claude-code-checklist.md). Example: [`examples/claude-code.sh`](examples/claude-code.sh).

---

## Library use

```go
import (
    gateway "github.com/inja-online/llm-gateway"
    "github.com/inja-online/llm-gateway/config"
    "github.com/inja-online/llm-gateway/hooks/jsonl"
)

cfg, err := config.Load("gateway.yaml")
// ...
hook, _ := jsonl.New(cfg.Hooks.JSONL.Output)
h, err := gateway.New(cfg, gateway.WithHook(hook))
// http.ListenAndServe(cfg.Listen, h)
```

Inject Vertex/ADC tokens with `proxy.Server.SetTokenSource` when embedding the package.

---

## Architecture

```
cmd/gateway          → binary, flags, graceful shutdown
config/              → YAML, capabilities, edge_auth, body limit
proxy/               → HTTP/WS routing, passthrough, translation orchestration
canonical/           → dialect-neutral chat/image/video/audio/realtime types
ingress/{openai,anthropic,google}/
egress/{openai,anthropic,google}/
hooks/{jsonl,webhook}/
```

**Invariant:** same-family traffic prefers byte passthrough; translation only when dialects differ.

---

## Deploy

| Path | Use |
|---|---|
| [`Dockerfile`](Dockerfile) | Distroless multi-stage build |
| [`docker-compose.yml`](docker-compose.yml) | Local stack |
| [`deploy/k8s/gateway.yaml`](deploy/k8s/gateway.yaml) | Sample Deployment/Service |
| [Release workflow](.github/workflows/release.yml) | Tag `v*` → multi-arch binaries |

Set secrets via env (`api_key_env` / `keys_env`); mount config read-only.

---

## Development & CI

```bash
go test ./...
go test -race ./...
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1
go vet ./...
docker build -t llm-gateway:dev .
```

**CI** (push/PR): build, vet, `go test -race -count=1 ./...` (air-gapped `httptest` only — **no** `-tags live`), coverage **≥ 90%**, binary smoke, Docker healthz.

Hermetic dialect anchors: [docs/sdk-compatibility-matrix.md](docs/sdk-compatibility-matrix.md).

**Docs site** (Astro, GitHub Pages):

```bash
cd website && npm install && npm run dev   # http://localhost:4321/llm-gateway/
```

Source: [`website/`](website/) · workflow: [`.github/workflows/docs.yml`](.github/workflows/docs.yml).

**Release:** `git tag vX.Y.Z && git push origin vX.Y.Z`

---

## Roadmap

Shipped: multi-dialect chat fidelity, media/audio, Responses/Files/Batches, Realtime/Live passthrough, models capabilities, edge auth, AGPL-3.0.

Possible follow-ups:

- Hardened production `wss`/TLS dial edge cases for realtime
- Optional full Realtime ↔ Live IR bridge (today: fail-closed)
- Deeper cross-dialect image/video generation translation
- Richer Prometheus histograms/labels beyond low-cardinality counters

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (modality checklist + layout).  
Security: [SECURITY.md](SECURITY.md) · [docs/security-multipart-review.md](docs/security-multipart-review.md).  
Changelog: [CHANGELOG.md](CHANGELOG.md).

---

## License

[GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE) © 2026 [inja-online](https://github.com/inja-online)

This project is free software under the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

If you run a **modified** version of this software as a network service (for example a hosted LLM gateway), the AGPL requires that you offer the corresponding source code of that modified version to users of the service. See [LICENSE](LICENSE) for the full terms.
