<p align="center">
  <img src="docs/assets/logo.jpg" alt="Inja LLM Gateway" width="128" height="128" />
</p>

<h1 align="center">Inja LLM Gateway</h1>

<p align="center">
  <strong>Small, dependency-free LLM API gateway</strong><br/>
  OpenAI + Anthropic + Gemini dialects · multi-provider routing · usage hooks<br/>
  One static binary — laptop, Docker, or Kubernetes
</p>

<p align="center">
  <a href="https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml"><img src="https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://github.com/inja-online/llm-gateway/actions/workflows/release.yml"><img src="https://github.com/inja-online/llm-gateway/actions/workflows/release.yml/badge.svg" alt="Release" /></a>
  <a href="https://github.com/inja-online/llm-gateway/releases"><img src="https://img.shields.io/github/v/release/inja-online/llm-gateway?include_prereleases&sort=semver&display_name=tag&label=release&color=blue" alt="Latest release" /></a>
  <a href="https://pkg.go.dev/github.com/inja-online/llm-gateway"><img src="https://pkg.go.dev/badge/github.com/inja-online/llm-gateway.svg" alt="Go Reference" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-yellow.svg" alt="MIT License" /></a>
  <br/>
  <a href="https://go.dev/dl/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go 1.25+" /></a>
  <a href=".github/workflows/ci.yml"><img src="https://img.shields.io/badge/coverage-%E2%89%A590%25-brightgreen" alt="Coverage ≥90%" /></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/deps-yaml.v3%20only-informational" alt="yaml.v3 only" /></a>
  <a href="#architecture"><img src="https://img.shields.io/badge/architecture-stateless-success" alt="Stateless" /></a>
  <br/>
  <a href="#quickstart"><img src="https://img.shields.io/badge/OS-linux%20%7C%20macOS%20%7C%20Windows-lightgrey" alt="Platforms" /></a>
  <a href="Dockerfile"><img src="https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white" alt="Docker ready" /></a>
  <a href="deploy/k8s/gateway.yaml"><img src="https://img.shields.io/badge/Kubernetes-ready-326CE5?logo=kubernetes&logoColor=white" alt="Kubernetes ready" /></a>
  <a href="https://github.com/inja-online/llm-gateway/stargazers"><img src="https://img.shields.io/github/stars/inja-online/llm-gateway?style=social" alt="GitHub stars" /></a>
</p>

<p align="center">
  <a href="#quickstart">Quickstart</a> ·
  <a href="#features">Features</a> ·
  <a href="#http-api">API</a> ·
  <a href="#configuration">Config</a> ·
  <a href="#deploy">Deploy</a> ·
  <a href="CONTRIBUTING.md">Contributing</a> ·
  <a href="SECURITY.md">Security</a>
</p>

---

Clients speak **OpenAI**, **Anthropic**, or **Gemini (native)**. The gateway routes to any upstream (OpenAI, Anthropic, Google, DeepSeek, xAI, Moonshot, OpenRouter, vLLM, …), translates dialects when needed, and emits **exactly one usage event per chat request** — no database; optional edge auth only.

```
  OpenAI SDK / Anthropic SDK / Gemini client / Claude Code / curl
                            │
                            ▼
                    ┌───────────────┐
                    │  llm-gateway  │──► JSONL (stdout) / webhook / Go hook
                    └───────┬───────┘
                            │
         ┌──────────┬───────┼────────┬────────────┐
         ▼          ▼       ▼        ▼            ▼
      OpenAI   Anthropic  Google  OpenAI-compat  …
                              native   (xAI, …)
```

| | |
|---|---|
| **Stateless** | No DB, sessions, or sticky routing — scale with identical replicas |
| **Cloud-native** | Distroless Docker, K8s manifests, SIGTERM drain, env overrides |
| **Local-first** | Single binary on macOS, Linux, or Windows; `docker compose up` |
| **Deps** | Runtime: `gopkg.in/yaml.v3` only |
| **Module** | [`github.com/inja-online/llm-gateway`](https://pkg.go.dev/github.com/inja-online/llm-gateway) |
| **Binary** | `llm-gateway` |

---

## Table of contents

- [Status](#status)
- [Features](#features)
- [Google / Gemini (both APIs)](#google--gemini-both-apis)
- [Quickstart](#quickstart)
- [Client examples](#client-examples)
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

**Also:** [docs/compatibility-matrix.md](docs/compatibility-matrix.md) · [CHANGELOG.md](CHANGELOG.md) · [docs/deprecation-policy.md](docs/deprecation-policy.md) · [docs/claude-code-checklist.md](docs/claude-code-checklist.md)

---

## Status

**Working** (early project). The HTTP surface and config schema below are the supported contract — build against them.

| Client dialect | Upstream | Path |
|---|---|---|
| OpenAI `POST /v1/chat/completions` | `openai` / `openai_compat` | **passthrough** |
| OpenAI | `anthropic` | **translated** |
| OpenAI | `google` (native Gemini) | **translated** |
| Anthropic `POST /v1/messages` | `anthropic` | **passthrough** |
| Anthropic | `openai` / `openai_compat` | **translated** |
| Anthropic | `google` | **translated** |
| Google `POST /v1beta/models/{model}:generateContent` | `google` | **passthrough** |
| Google stream `:streamGenerateContent` | `google` | **passthrough** |
| Google `:countTokens` | `google` | **passthrough** |
| Google | `openai` / `openai_compat` / `anthropic` | **translated** |

Also shipped:

- Streaming + non-streaming, tools, multimodal **input** images, system prompts
- OpenAI-compatible **image generation** (`/v1/images/*`) and **video generation** (`/v1/videos`)
- Native Gemini model discovery (`GET /v1beta/models`, `GET /v1beta/models/{model}`)
- **Embeddings**: `POST /v1/embeddings` (OpenAI-compat passthrough + OpenAI→Google map); native Gemini `:embedContent` / `:batchEmbedContents`
- Dialect-shaped errors
- One usage event per chat / media request (JSONL / webhook / Go hook)
- `POST /v1/messages/count_tokens`, `GET /v1/models`, `GET /healthz`
- `POST /v1/messages/count_tokens` (Anthropic proxy, Google `:countTokens` translate, or estimate), `GET /healthz`
- One usage event per chat / media / embedding request (JSONL / webhook / Go hook)
- OpenAI **Responses** API (`/v1/responses`, streaming SSE, GET/DELETE by id)
- OpenAI **Files** API proxy (`/v1/files*`) and Anthropic **Files** (same paths + `anthropic-version`); **Moderations** (`/v1/moderations`)
||||||| f28ae33
- OpenAI **Files** API proxy (`/v1/files*`) and **Moderations** (`/v1/moderations`)
- OpenAI **Files** API proxy (`/v1/files*`) and **Moderations** (`/v1/moderations`)
- Anthropic **Message Batches** proxy (`/v1/messages/batches*`; upstream-owned; nested model rewrite)
- Realtime WebSocket skeleton (`/v1/realtime`, Google Live path) with session limits
- Dialect-shaped errors; rate-limit + OpenAI org/project header passthrough
- One usage event per chat / media / responses / files request (JSONL / webhook / Go hook)
- `POST /v1/messages/count_tokens`, `GET /healthz`
- Graceful shutdown, Docker, Kubernetes, multi-arch releases

---

## Features

| Area | What you get |
|---|---|
| **Triple ingress** | OpenAI, Anthropic (Claude Code), and native Gemini `generateContent` |
| **Media generation** | OpenAI-compat `images/*` + `videos` (OpenAI, Gemini OpenAI-compat, …) |
| **Multi-provider egress** | Native Anthropic + native Gemini + any OpenAI-compatible host (xAI, Moonshot, DeepSeek, …) |
| **Cross-dialect translation** | OpenAI ↔ Anthropic ↔ Google **chat** when client and upstream disagree |
| **Passthrough-first** | Same dialect → near-verbatim bytes (full fidelity) |
| **Usage metering** | JSONL (stdout/file), async webhook, or in-process Go hook — one event per chat request |
| **Ops** | One YAML file, `GATEWAY_LISTEN` / `GATEWAY_CONFIG`, `/healthz`, 30s SIGTERM drain |
| **Ship anywhere** | Multi-arch release binaries, distroless Docker, K8s sample manifests |

---

## Google / Gemini (both APIs)

Google exposes **two** wire formats. The gateway supports **both as egress** and accepts clients for **both** (native as its own dialect; OpenAI-compat via the shared OpenAI dialect — same bytes).

| Gemini API | Wire format | Ingress (client → gateway) | Egress (gateway → Google) |
|---|---|---|---|
| **Native** | `generateContent` / `streamGenerateContent` / `countTokens` | **Yes** — dedicated dialect `POST /v1beta/models/{model}:…` (`ingress/google`) | **Yes** — `kind: google` (`egress/google`), `x-goog-api-key` |
| **Native discovery** | `GET /v1beta/models` (+ `/{model}`) | **Yes** — passthrough to `kind: google` (`?provider=` or `defaults.google_dialect`) | **Yes** — `x-goog-api-key` |
| **OpenAI-compat** | Chat Completions | **Yes** — OpenAI dialect `POST /v1/chat/completions` (identical wire; no separate Google route) | **Yes** — `kind: openai_compat` + Gemini OpenAI base, Bearer |

Template entries:

```yaml
# Native Gemini (dialect + provider)
google:
  kind: google
  base_url: "https://generativelanguage.googleapis.com/v1beta"
  # api_key_env: GEMINI_API_KEY

# Gemini OpenAI-compatible (provider; clients use OpenAI dialect)
google_openai:
  kind: openai_compat
  base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
  # api_key_env: GEMINI_API_KEY
```

| Client wants | Call the gateway with | Route model like |
|---|---|---|
| Native Gemini SDK / REST | `/v1beta/models/…:generateContent` | bare id → `defaults.google_dialect`, or `provider/model` in body |
| OpenAI SDK against Gemini | `/v1/chat/completions` | `google_openai/gemini-2.0-flash` (passthrough) or `google/gemini-…` (translated to native) |
| Claude Code / Anthropic SDK against Gemini | `/v1/messages` | `google/…` or `google_openai/…` (translated) |

---

## Quickstart

### 1. Binary (macOS / Linux / Windows)

```bash
git clone https://github.com/inja-online/llm-gateway.git
cd llm-gateway
go build -o llm-gateway ./cmd/gateway
# Windows: go build -o llm-gateway.exe ./cmd/gateway

cp gateway.example.yaml gateway.yaml
# edit providers / keys / hooks
./llm-gateway -config gateway.yaml
# Windows: .\llm-gateway.exe -config gateway.yaml
```

Release binaries (when tagged): GitHub **Releases** → `linux`/`darwin` (`amd64`, `arm64`) and `windows` (`amd64`), plus checksums.

| Env | Purpose |
|---|---|
| `GATEWAY_CONFIG` | Default `-config` path |
| `GATEWAY_LISTEN` | Bind address (overrides YAML `listen`) |

Default listen: **`:8787`**. SIGINT / SIGTERM drain in-flight work for up to **30s**.

### 2. Docker

```bash
docker compose up --build
curl http://localhost:8787/healthz
```

```bash
docker build -t llm-gateway .
docker run --rm -p 8787:8787 \
  -v "$PWD/gateway.yaml:/config/gateway.yaml:ro" \
  -e GATEWAY_LISTEN=:8787 \
  llm-gateway
```

### 3. Kubernetes

```bash
# Set the Deployment image to your registry build of the Dockerfile, then:
kubectl apply -f deploy/k8s/gateway.yaml
kubectl -n llm-gateway port-forward svc/llm-gateway 8787:8787
```

Replicas share nothing. Prefer `hooks.jsonl.output: stdout` + log shipping, or `hooks.webhook.url`.

### Minimal config

```yaml
listen: ":8787"
providers:
  openai:        { kind: openai,         base_url: "https://api.openai.com/v1" }
  anthropic:     { kind: anthropic,      base_url: "https://api.anthropic.com/v1" }
  google:        { kind: google,         base_url: "https://generativelanguage.googleapis.com/v1beta" }
  google_openai: { kind: openai_compat,  base_url: "https://generativelanguage.googleapis.com/v1beta/openai" }
  deepseek:      { kind: openai_compat,  base_url: "https://api.deepseek.com" }
  xai:           { kind: openai_compat,  base_url: "https://api.x.ai/v1" }
  moonshot:      { kind: openai_compat,  base_url: "https://api.moonshot.ai/v1" }
defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google
aliases:
  fast: deepseek/deepseek-chat
  grok: xai/grok-3
hooks:
  jsonl: { output: stdout }
```

Full sample (xAI, Moonshot, Z.AI, Groq, Qwen, …): [`gateway.example.yaml`](gateway.example.yaml).

---

## Client examples

**OpenAI SDK** (any OpenAI-compatible provider via the gateway):

```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key="<key for target provider>")
r = client.chat.completions.create(
    model="deepseek/deepseek-chat",
    messages=[{"role": "user", "content": "hi"}],
)
```

**Anthropic SDK** (including non-Anthropic models — gateway translates). Use an **alias** or `provider/model` id; model discovery (if any client probes it) is the OpenAI-shaped `GET /v1/models`, not an Anthropic twin:

```python
from anthropic import Anthropic
client = Anthropic(base_url="http://localhost:8787", api_key="<key for target provider>")
r = client.messages.create(
    model="fast",  # or "deepseek/deepseek-chat"
    max_tokens=100,
    messages=[{"role": "user", "content": "hi"}],
)
```

**Gemini native** (path-style `generateContent`):

```bash
curl "http://localhost:8787/v1beta/models/gemini-2.0-flash:generateContent" \
  -H "Content-Type: application/json" \
  -H "x-goog-api-key: $GEMINI_API_KEY" \
  -d '{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}'
```

**Gemini OpenAI-compat** (same as OpenAI SDK, different provider name):

```python
client = OpenAI(base_url="http://localhost:8787/v1", api_key=os.environ["GEMINI_API_KEY"])
r = client.chat.completions.create(model="google_openai/gemini-2.0-flash", messages=[...])
```

**Shell / PowerShell**

```bash
KEY=sk-... MODEL=deepseek/deepseek-chat ./examples/curl-openai.sh
KEY=sk-ant-... ./examples/claude-code.sh
```

```powershell
$env:KEY = "sk-..."
.\examples\curl-openai.ps1
```

With JSONL → stdout, each chat request logs one line:

```json
{"request_id":"req_1a2b3c","time":"2026-07-18T12:00:00Z","dialect_in":"openai","provider":"deepseek","model":"deepseek/deepseek-chat","upstream_model":"deepseek-chat","tokens_in":12,"tokens_out":5,"estimated":false,"stream":false,"status":"ok","http_status":200,"latency_ms":812,"key_hash":"9f8e7d6c5b4a"}
```

---

## HTTP API

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/chat/completions` | OpenAI dialect (also Gemini OpenAI-compat clients) |
| `POST` | `/v1/messages` | Anthropic Messages dialect |
| `POST` | `/v1/embeddings` | OpenAI embeddings (passthrough; OpenAI→Google map when `kind: google`) |
| `POST` | `/v1beta/models/{model}:generateContent` | Gemini **native** dialect |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | Gemini native streaming (upstream `?alt=sse`) |
| `POST` | `/v1beta/models/{model}:countTokens` | Gemini native token count (passthrough; no usage event) |
| `GET` | `/v1beta/models` | List Gemini models (`?provider=` or `defaults.google_dialect`) |
| `GET` | `/v1beta/models/{model}` | Get one Gemini model |
| `POST` | `/v1beta/models/{model}:embedContent` | Gemini native embeddings (single) |
| `POST` | `/v1beta/models/{model}:batchEmbedContents` | Gemini native embeddings (batch) |
| `POST` | `/v1/responses` | OpenAI Responses create (passthrough; `stream:true` SSE) |
| `GET` | `/v1/responses/{id}` | Retrieve stored response (proxy only; no gateway storage) |
| `DELETE` | `/v1/responses/{id}` | Delete stored response upstream |
| `POST` | `/v1/files` | Upload file (multipart; OpenAI or Anthropic via `anthropic-version`) |
| `GET` | `/v1/files` | List files |
| `GET` | `/v1/files/{id}` | Retrieve file metadata |
| `DELETE` | `/v1/files/{id}` | Delete file upstream |
| `GET` | `/v1/files/{id}/content` | Download file content (streamed) |
| `POST` | `/v1/messages/batches` | Create Anthropic Message Batch (nested model rewrite) |
| `GET` | `/v1/messages/batches` | List batches |
| `GET` | `/v1/messages/batches/{id}` | Retrieve batch status |
| `POST` | `/v1/messages/batches/{id}/cancel` | Cancel a batch |
| `GET` | `/v1/messages/batches/{id}/results` | Download batch results (JSONL; upstream-owned) |
| `POST` | `/v1/moderations` | OpenAI Moderations passthrough |
| `POST` | `/v1/images/generations` | Image generation (OpenAI-compat passthrough) |
| `POST` | `/v1/images/edits` | Image edits (JSON or multipart passthrough) |
| `POST` | `/v1/images/variations` | Image variations (passthrough) |
| `POST` | `/v1/videos` | Video generation job create (OpenAI / Gemini OpenAI-compat) |
| `GET` | `/v1/videos/{id}` | Video job status (`?provider=` or `defaults.openai_dialect`) |
| `GET` | `/v1/realtime` | OpenAI Realtime WebSocket upgrade (same-protocol passthrough) |
| `GET` | `/v1beta/models/{model}:bidiGenerateContent` | Google Live WebSocket (same-protocol passthrough) |
| `POST` | `/v1/conversations` (+ `/{id}`, items) | **501 stub** — Conversations API not implemented (see below) |
| `POST` | `/v1/messages/count_tokens` | Token count (proxy or estimate) |
| `GET` | `/v1/models` | OpenAI-shaped catalog (aliases + alias targets from config) |
| `GET` | `/v1/models/{id}` | Retrieve one catalog entry (supports `provider/model` ids) |
| `POST` | `/v1/messages/count_tokens` | Token count (proxy, Google translate, or estimate) |
| `GET` | `/healthz` | Liveness / readiness: `{"status":"ok"}` |

There is **no** separate `/v1beta/openai/…` ingress: Gemini’s OpenAI-compat API is the same Chat Completions shape, so clients use `/v1/chat/completions` and a provider such as `google_openai`.

### Model discovery (`GET /v1/models`)

OpenAI SDKs call `models.list` / `models.retrieve` on connect. The gateway serves a **config-derived** catalog (no live upstream fan-out):
- **Public ids:** every `aliases` key, plus every unique alias target as stored (e.g. `deepseek/deepseek-chat`)
- **Shape:** `{"object":"list","data":[{"id","object":"model","created","owned_by"}]}`
- **`owned_by`:** `"llm-gateway"` for alias keys; provider name for `provider/model` targets
- **No usage event** (discovery only)
- Missing id → OpenAI error envelope `404`
**Anthropic models surface:** there is **no** Anthropic-shaped models twin (`/v1/models` under Messages-style envelope). Anthropic / Claude Code clients use aliases or `provider/model` ids on `POST /v1/messages` (and OpenAI-shaped discovery if needed). No fake Anthropic list wire is planned unless a product need appears.
```bash
curl -s http://localhost:8787/v1/models
curl -s http://localhost:8787/v1/models/fast
curl -s http://localhost:8787/v1/models/deepseek/deepseek-chat
```
### Embeddings
| Route | Providers | Notes |
|---|---|---|
| `POST /v1/embeddings` | `openai`, `openai_compat` | Model rewrite + passthrough to `{base}/embeddings` |
| `POST /v1/embeddings` | `google` | Translates to `:embedContent` (single string) or `:batchEmbedContents` (array); response remapped to OpenAI list shape |
| `POST /v1beta/models/{model}:embedContent` | `google` | Native Gemini passthrough |
| `POST /v1beta/models/{model}:batchEmbedContents` | `google` | Native Gemini batch passthrough |
Usage events use `modality: "embedding"` (prompt tokens when upstream reports them). Anthropic providers are rejected with an OpenAI error envelope (`501`).
```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key=os.environ["OPENAI_API_KEY"])
# OpenAI-family passthrough
client.embeddings.create(model="openai/text-embedding-3-small", input="hello")
# Or map to native Gemini when the resolved provider is kind: google
# client.embeddings.create(model="google/gemini-embedding-001", input=["a","b"])
### Conversations API (decision: **stub 501**)

OpenAI **Conversations** (and Assistants threads) are **not implemented**. The gateway is **stateless** and does not store conversation history.

| Decision | Detail |
|---|---|
| **Chosen** | **Stub 501** on common Conversations routes so SDKs get an OpenAI-shaped error instead of a bare 404 |
| Routes | `POST/GET /v1/conversations`, `GET/POST/DELETE /v1/conversations/{id}`, and nested paths (e.g. `…/items`) |
| Error | HTTP **501**, `error.type` = `not_implemented`, message points to Responses + client-side state / Files |
| **Not chosen** | Full passthrough (would imply gateway-side or opaque upstream state without clear ROI) or silent skip (bare 404 confuses SDKs) |

**Alternative:** use **`POST /v1/responses`** (OpenAI-family passthrough) with **client-side conversation state**, and/or the **Files** API for provider-stored assets. Assistants `/v1/threads` is likewise not exposed (no stub; same alternative).

### Responses API
OpenAI-family only (`kind: openai` or `openai_compat`). Same model routing as chat (`aliases` / `provider/model` / `defaults.openai_dialect`).
| Call | Notes |
|---|---|
| `POST /v1/responses` | Rewrites `model`; preserves unknown JSON fields; one usage event (`input_tokens` / `output_tokens` when present) |
| `stream: true` | Byte-faithful SSE of typed events (`response.created`, `response.completed`, …); usage from completed event |
| `GET` / `DELETE /v1/responses/{id}` | Provider: `?provider=` **or** `X-Provider` **or** `defaults.openai_dialect` — gateway does **not** store bodies |
client = OpenAI(base_url="http://localhost:8787/v1", api_key="sk-...")
r = client.responses.create(model="openai/gpt-4o", input="hello")
### Files API
**Files live on the upstream provider**, not on the gateway (no disk spool beyond the in-flight request body; global body limit **32 MiB**). Multipart upload `Content-Type` (including boundary) is forwarded intact.

Shared paths (`POST/GET/DELETE /v1/files*`). Dialect is selected by header:

| Client signal | Dialect | Provider kinds | Provider selection |
|---|---|---|---|
| **No** `anthropic-version` | OpenAI Files | `openai`, `openai_compat` only | `?provider=` → `X-Provider` → `defaults.openai_dialect` |
| **`anthropic-version` present** | Anthropic Files | `kind: anthropic` only (fail closed) | `?provider=` → `X-Provider` → `defaults.anthropic_dialect` |

**Anthropic Files** (beta on Anthropic’s side) uses the same path shape as OpenAI (`/v1/files`, `/v1/files/{id}`, `/v1/files/{id}/content`). The gateway:

- Forwards client `anthropic-version` when set; Anthropic auth still injects default `2023-06-01` when the client omits a value after auth rewrite (same policy as Messages)
- Forwards `anthropic-beta` **as-is** (including comma-separated / unknown future values such as `files-api-2025-04-14`); **no beta allowlist** — new betas work without a gateway release
- Does **not** invent or force a beta string; clients/SDKs that call Files should send the beta Anthropic requires
- Emits Anthropic-shaped errors for gateway failures on the Anthropic dialect path

Usage: one operational event per call (`estimated: true`, zero tokens) for both dialects.

```bash
# OpenAI Files
curl -s http://localhost:8787/v1/files -H "Authorization: Bearer $OPENAI_API_KEY" \
  -F purpose=assistants -F file=@doc.txt

# Anthropic Files (requires anthropic-version; forward anthropic-beta for Files beta)
curl -s http://localhost:8787/v1/files \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "anthropic-beta: files-api-2025-04-14" \
  -F file=@doc.pdf
```
||||||| f28ae33
OpenAI-family proxy. **Files live on the upstream provider**, not on the gateway (no disk spool beyond the in-flight request body; global body limit **32 MiB**).
Provider selection (no model field): `?provider=` → `X-Provider` → `defaults.openai_dialect`.
Usage: one operational event per call (`estimated: true`, zero tokens).
OpenAI-family proxy. **Files live on the upstream provider**, not on the gateway (no disk spool beyond the in-flight request body; global body limit **32 MiB**).
Provider selection (no model field): `?provider=` → `X-Provider` → `defaults.openai_dialect`.
Usage: one operational event per call (`estimated: true`, zero tokens).
### Anthropic Message Batches
Anthropic-kind only (`kind: anthropic`). **Batches and results live on the upstream Anthropic provider** — the gateway does **not** store job state or result bodies (pure proxy).

Provider selection (no top-level model): `?provider=` → `X-Provider` → `defaults.anthropic_dialect`.

| Call | Notes |
|---|---|
| `POST /v1/messages/batches` | Rewrites nested `requests[].params.model` (aliases / `provider/model` → bare upstream id); all models must resolve to the same batch provider |
| `GET /v1/messages/batches` | List (query passthrough; `provider` stripped) |
| `GET /v1/messages/batches/{id}` | Retrieve status |
| `POST /v1/messages/batches/{id}/cancel` | Cancel |
| `GET /v1/messages/batches/{id}/results` | JSONL results stream from upstream |

Auth/headers match Anthropic chat: client `x-api-key` (or `api_key_env`), default `anthropic-version` when unset, client `anthropic-version` / `anthropic-beta` forwarded (no beta allowlist).

Usage: **create** emits one event (`estimated: true`, coarse `tokens_in` from body size); **list/get/cancel/results** emit light operational events only (`estimated: true`, zero tokens) so polling does not spam metering.
### Moderations
`POST /v1/moderations` — OpenAI-family only; rewrites `model` when present; otherwise uses default OpenAI-family provider.
### Realtime (WebSocket)

Same-protocol **passthrough** only. The optional **OpenAI Realtime ↔ Google Live bridge is not implemented** and is deferred to a future milestone (fail closed; no half-bridge).

| Path | Provider | Capability | Mode |
|---|---|---|---|
| `GET /v1/realtime?model=…` | `openai` / `openai_compat` with `capabilities.realtime` | OpenAI Realtime | Passthrough |
| `GET /v1beta/models/{model}:bidiGenerateContent` | `kind: google` with `realtime` | Google Live | Passthrough |

| Cross-protocol attempt | Result |
|---|---|
| OpenAI Realtime ingress → `kind: google` provider | **501** `unsupported_realtime_bridge` — use native Live URL for Google |
| Google Live ingress → `openai` / `openai_compat` provider | **501** `unsupported_realtime_bridge` — use `/v1/realtime` for OpenAI |
| Anthropic (any live path) | Unsupported (no Anthropic WebSocket dialect) |

Process limits from config (`realtime.max_sessions`, `realtime.max_session_minutes`). One usage event on session end (`modality=realtime`, `transport=websocket`, `media.unit_kind=session_minute`).

**Still open (same-protocol):** production TLS/`wss` dial and fuller protocol edge cases. Hermetic tests cover upgrade + limits + capability deny + bridge fail-closed.

**Realtime bridge drop list (unmapped):** full IR event mapping is unmapped / won't ship now — no OpenAI↔Live event translation, no audio format conversion, no tool-call remapping across protocols. Documented for drop-list policy: fail closed rather than re-encode as a wrong type.
### Image & video generation

Chat multimodal **inputs** (image URL / base64 in messages) are already part of chat translation. **Generation** APIs are separate OpenAI-compatible routes:

| API | Providers | Notes |
|---|---|---|
| Images | `openai` (on by default); `openai_compat` **with** `capabilities.image_gen: true` | Rewrite `model`; fail closed if modality unsupported; usage `modality=image_gen` |
| Videos | `openai` (on by default); `openai_compat` **with** `capabilities.video_gen: true` | Create `POST` bills `video_second`; poll `GET /v1/videos/{id}?provider=…` is operational (`units=0`) |
| Native Gemini image-in-chat | `kind: google` | Via `generateContent` image models / modalities on the Google dialect |

**Capability defaults:** `kind: openai` and `kind: google` allow media; `kind: anthropic` is text-only; `kind: openai_compat` is **text-only until you opt in** (prevents silent routing to hosts that 404). Example for Gemini OpenAI-compat (`gateway.example.yaml`):

```yaml
google_openai:
  kind: openai_compat
  base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
  capabilities:
    text: true
    image_gen: true
    video_gen: true
```

Without opt-in, image/video routes return an OpenAI error envelope with `type: unsupported_provider_capability` and **never** call upstream.

### Voice (HTTP audio — TTS / STT)

| Dialect | TTS | STT | Notes |
|---|---|---|---|
| **OpenAI** | `POST /v1/audio/speech` | `POST /v1/audio/transcriptions`, `/translations` | OpenAI-family: model rewrite + binary/multipart **byte-passthrough**. `kind: google`: TTS translates to Gemini `generateContent` with `responseModalities=["AUDIO"]`; response is decoded audio bytes (base64 decode only — **no codec re-encode**). |
| **Anthropic gateway** | same `/v1/audio/speech` | same `/v1/audio/transcriptions` (+ `/translations`) | Requires **`anthropic-version`** header (disambiguates dialect). Errors use Anthropic envelope. Translates to OpenAI TTS/STT or Google TTS; pure `kind: anthropic` fails closed (`unsupported_provider_capability` — Anthropic has no native TTS/STT). |
| **Google** | `POST /v1beta/models/{model}:generateSpeech` | — | Gateway Media Contract path. `kind: google` → real Gemini TTS via `:generateContent` + AUDIO modality (JSON with base64 `inlineData`). `openai` / `openai_compat` → OpenAI speech binary, wrapped as generateContent-shaped JSON for Google clients. |

**Capability:** `audio_speech` / `audio_transcribe`. Defaults: on for `openai` + `google`; **off** for `anthropic` and `openai_compat` (opt in). Fail closed before any upstream call.

**Fidelity:** Same-family TTS responses and multipart STT bodies are byte-equal passthrough (Content-Type + boundary preserved). Re-encode only when translating formats is required by the dialect contract (today: base64 wrap/unwrap for Google↔binary only — no mp3↔pcm conversion). Multipart body limit **32 MiB** (`maxBodyBytes`).

```bash
# OpenAI TTS
curl -sS http://localhost:8787/v1/audio/speech \
  -H "Authorization: Bearer $OPENAI_API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"tts-1","input":"Hello","voice":"alloy"}' -o out.mp3

# Anthropic-gateway TTS (routes to openai dialect default / provider prefix)
curl -sS http://localhost:8787/v1/audio/speech \
  -H "anthropic-version: 2023-06-01" -H "x-api-key: $KEY" -H "Content-Type: application/json" \
  -d '{"model":"openai/tts-1","input":"Hello","voice":"alloy"}' -o out.mp3

# Google-shaped TTS (Gemini TTS models)
curl -sS "http://localhost:8787/v1beta/models/gemini-2.5-flash-preview-tts:generateSpeech" \
  -H "x-goog-api-key: $GEMINI_API_KEY" -H "Content-Type: application/json" \
  -d '{"text":"Hello","voice":"Kore"}'
```

Usage events: `modality=audio_speech` (`media.unit_kind=audio_character`) or `audio_transcribe` (`audio_minute`).

```python
# Image gen via Gemini OpenAI-compat (requires capabilities.image_gen on google_openai)
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key=os.environ["GEMINI_API_KEY"])
img = client.images.generate(model="google_openai/gemini-2.5-flash-image", prompt="a sheepadoodle in a cape")

# Video job (poll status)
# POST /v1/videos  {"model":"google_openai/veo-3.1-generate-preview","prompt":"..."}
# GET  /v1/videos/{id}?provider=google_openai
```

Unknown routes → standard 404.

**Limits**

| Limit | Value |
|---|---|
| Request / response body | 32 MiB |
| Overall proxy timeout | none (streams may be long-lived) |
| Upstream response header wait | 60s |
| `count_tokens` / `:countTokens` / models GET | 15s |
| Shutdown drain | 30s |

Client disconnect cancels the upstream context.

### `count_tokens`

| Resolved provider | Behavior |
|---|---|
| `anthropic` | Proxy to real `…/messages/count_tokens`; fall back to estimate on failure |
| `google` | Translate Anthropic body → Gemini `:countTokens`; map `totalTokens` → `{input_tokens}`; fall back to estimate on failure |
| other | Local estimate only (~1 token / 4 chars of text & tool schema) |

Estimate is for client compatibility (e.g. Claude Code), **not billing**. No usage event is emitted.

Native Gemini clients can also call `POST /v1beta/models/{model}:countTokens` directly (Google-shaped body/response; also no usage event).

### Google model discovery

`GET /v1beta/models` and `GET /v1beta/models/{model}` passthrough to a `kind: google` provider. Choose the provider with `?provider=NAME` or `defaults.google_dialect`. Auth uses `x-goog-api-key` (or `api_key_env`). No usage event.

---

## Model routing

Public `model` resolves in order:

1. **`aliases`** — exact match (`fast` → `deepseek/deepseek-chat`)
2. **`provider/model`** — first segment must be a configured provider name
3. **Bare id** — dialect default (`defaults.openai_dialect`, `defaults.anthropic_dialect`, or `defaults.google_dialect`)

Missing default or unknown provider → **404** (dialect error envelope).

| Client `model` | Dialect | Provider | Upstream model |
|---|---|---|---|
| `deepseek/deepseek-chat` | either | `deepseek` | `deepseek-chat` |
| `fast` | either | `deepseek` | `deepseek-chat` |
| `gpt-4o` | OpenAI | `openai` (default) | `gpt-4o` |
| `claude-sonnet-4-20250514` | Anthropic | `anthropic` (default) | `claude-sonnet-4-20250514` |
| `gemini-2.0-flash` (path) | Google | `google` (default) | `gemini-2.0-flash` |
| `google/gemini-2.0-flash` | OpenAI | `google` | `gemini-2.0-flash` |

---

## Configuration

Single YAML file. Unknown fields are rejected.

| Field | Required | Description |
|---|---|---|
| `listen` | no | Bind address; default `:8787` |
| `providers` | yes | Map of name → provider (≥1) |
| `providers.<n>.kind` | yes | `openai` \| `openai_compat` \| `anthropic` \| `google` |
| `providers.<n>.base_url` | yes | Origin **with version prefix**; trailing `/` trimmed |
| `providers.<n>.api_key_env` | no | Env var; when set & non-empty, **replaces** client key |
| `providers.<n>.capabilities` | no | Override modality flags (`text`, `image_gen`, `video_gen`, `audio_*`, `realtime`). Nil → kind defaults (`openai_compat` = text only) |
| `providers.<n>.auth` | no | `api_key` (default) \| `adc` \| `service_account` \| `bearer` |
| `providers.<n>.service_account_file` | no | Operator path for SA JSON (document/mount; TokenSource applies tokens) |
| `providers.<n>.capabilities` | no | Override kind defaults (`image_gen`, `audio_transcribe`, …) |
| `defaults.openai_dialect` | no | Provider for bare models on OpenAI ingress |
| `defaults.anthropic_dialect` | no | Provider for bare models on Anthropic ingress |
| `defaults.google_dialect` | no | Provider for bare models on Gemini ingress |
| `aliases` | no | Public id → `provider/upstream-model` |
| `edge_auth.enabled` | no | Default `false`. When true, require edge key (see Auth) |
| `edge_auth.keys` | no | Inline edge keys (prefer env in production) |
| `edge_auth.keys_env` | no | Env var with **comma-separated** edge keys |
| `realtime.max_sessions` | no | Default `1024` (process-local WS cap when realtime lands) |
| `realtime.max_session_minutes` | no | Default `60` |
| `hooks.jsonl.output` | no | `stdout` \| `stderr` \| file path |
| `hooks.webhook.url` | no | Async POST of each usage event |
| `hooks.webhook.timeout` | no | Default `3s` |

### Provider kinds

| Kind | Upstream path | Auth | Typical use |
|---|---|---|---|
| `openai` | `{base_url}/chat/completions` | `Authorization: Bearer …` | OpenAI |
| `openai_compat` | same | same | DeepSeek, xAI, Moonshot, **Gemini OpenAI-compat**, vLLM, … |
| `anthropic` | `{base_url}/messages` | `x-api-key` + `anthropic-version: 2023-06-01` | Anthropic |
| `google` | `{base_url}/models/{model}:generateContent` (stream: `:streamGenerateContent?alt=sse`) | `x-goog-api-key` | **Gemini native** |

`base_url` examples:

- `https://api.openai.com/v1` → `…/v1/chat/completions`
- `https://api.anthropic.com/v1` → `…/v1/messages`
- `https://generativelanguage.googleapis.com/v1beta` → native Gemini (`kind: google`)
- `https://generativelanguage.googleapis.com/v1beta/openai` → Gemini OpenAI-compat (`kind: openai_compat`)
- `https://api.deepseek.com` / `https://api.x.ai/v1` / `https://api.moonshot.ai/v1` / … → OpenAI-compat

---

## Auth & keys

### Upstream credentials (always)

The gateway reads a client credential from:

1. `Authorization: Bearer <key>`, or
2. `x-api-key: <key>`, or
3. `x-goog-api-key: <key>`

…and forwards it using the provider’s auth scheme — unless replaced by `api_key_env` or ADC (below). **The upstream key must be valid for the target provider.** An OpenAI key routed to `anthropic/...` will be rejected upstream.

| Mode (`providers.<n>.auth`) | Behavior |
|---|---|
| `api_key` (default / empty) | Kind scheme: OpenAI Bearer, Anthropic `x-api-key`, Google `x-goog-api-key`. `api_key_env` replaces client key when set. |
| `bearer` | Always `Authorization: Bearer` (useful for some Google-compatible hosts). |
| `adc` / `service_account` | `Authorization: Bearer` from a **TokenSource** registered on the server (`SetTokenSource` in library mode). No Google cloud SDK is bundled; inject tokens from ADC, a refresh sidecar, or tests (`StaticTokenSource` / `CachingTokenSource`). |

Usage events include `key_hash`: first 12 hex chars of SHA-256 of the **upstream** credential (after `api_key_env` / token source) — correlate without storing secrets. Edge keys are not hashed separately.

### Optional edge auth (gateway gate)
By default the gateway **does not authenticate callers** (trusted network / external auth assumed). To require a shared secret at the edge:
```yaml
edge_auth:
  enabled: true
  keys_env: GATEWAY_EDGE_KEYS   # comma-separated, e.g. "key1,key2"
  # keys: ["dev-only"]          # optional inline
```
When enabled:
- Every route **except** `GET /healthz` requires a matching key via `Authorization: Bearer …` or `x-api-key`
- Missing/invalid → **401** OpenAI-shaped `authentication_error` / `invalid_edge_auth`
- Constant-time compare; keys are never logged
- **Distinct from upstream keys:** with `api_key_env` on providers, clients only need the edge key; the server substitutes the provider secret
See [SECURITY.md](SECURITY.md).
### Forwarded client headers
On upstream requests the gateway copies (when present): `HTTP-Referer`, `Referer`, `X-Title`, `OpenAI-Organization`, `OpenAI-Project`, `anthropic-beta`, and client `anthropic-version` (Anthropic egress may still set a default version when applying API-key auth). **`anthropic-beta` is not allowlisted** — unknown / future beta strings (including Files `files-api-2025-04-14`) are forwarded unchanged on Messages, count_tokens, and Anthropic Files.
---
## Provider notes
Full sample comments live in [`gateway.example.yaml`](gateway.example.yaml). Compatibility overview: [docs/compatibility-matrix.md](docs/compatibility-matrix.md).
### OpenRouter
| | |
|---|---|
| Kind | `openai_compat` |
| Base | `https://openrouter.ai/api/v1` |
| Models | `openrouter/<author>/<model>` (gateway strips the provider prefix upstream) |
| Auth | Bearer (`OPENROUTER_API_KEY`) |
| Headers | `HTTP-Referer`, `X-Title` forwarded (OpenRouter ranking / app attribution) |
| Body | Extra fields (`provider`, `plugins`, `route`, …) **passthrough** — not stripped |
| Media | `capabilities.image_gen: true` (etc.) required; defaults text-only for `openai_compat` |
```bash
curl http://localhost:8787/v1/chat/completions \
  -H "Authorization: Bearer $OPENROUTER_API_KEY" \
  -H "HTTP-Referer: https://example.com" \
  -H "X-Title: My App" \
  -H "Content-Type: application/json" \
  -d '{"model":"openrouter/anthropic/claude-3.5-sonnet","messages":[{"role":"user","content":"hi"}]}'
### xAI (Grok)
| Base | `https://api.x.ai/v1` |
| Alias | `grok` → `xai/grok-3` (example) |
| Routes | Chat Completions today; Responses / images when those gateway routes + host support exist (passthrough only; no xAI-specific IR) |
| Media | Opt-in `capabilities.image_gen` if using Imagine-style models via OpenAI images API |
```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key=os.environ["XAI_API_KEY"])
client.chat.completions.create(model="xai/grok-3", messages=[{"role":"user","content":"hi"}])
### Z.AI / Zhipu (GLM)
Pick the **regional** OpenAI-compat base that matches your key (confirm in current Z.AI / BigModel docs; bases change). Examples (2026-07):
| Region | Example `base_url` |
| International | `https://api.z.ai/api/paas/v4` |
| CN (BigModel) | `https://open.bigmodel.cn/api/paas/v4` |
`kind: openai_compat`, text-only unless capabilities set. Use one provider block per region/key.
### Qwen (DashScope)
OpenAI-compatible mode path includes `compatible-mode`:
| CN | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| International | `https://dashscope-intl.aliyuncs.com/compatible-mode/v1` |
Models: bare ids (`qwen-turbo`) with `defaults.openai_dialect: qwen`, or `qwen/qwen-turbo`, or aliases (`qwen-turbo: qwen/qwen-turbo`).
### Groq (STT-oriented routing)
Groq is `openai_compat` at `https://api.groq.com/openai/v1`. For **chat on provider A, STT on Groq**:
```yaml
providers:
  openai: { kind: openai, base_url: "https://api.openai.com/v1", api_key_env: OPENAI_API_KEY }
  groq:
    kind: openai_compat
    base_url: "https://api.groq.com/openai/v1"
    api_key_env: GROQ_API_KEY
    capabilities:
      text: true
      audio_transcribe: true
defaults:
  openai_dialect: openai
aliases:
  whisper-fast: groq/whisper-large-v3
```
Call STT with `model: groq/whisper-large-v3` or the alias. Multipart body limit **32 MiB**.
### Vertex AI (ADC)
  vertex:
    kind: google
    base_url: "https://REGION-aiplatform.googleapis.com/v1/projects/PROJECT/locations/REGION/publishers/google"
    auth: adc
    # service_account_file: /secrets/vertex-sa.json  # mount read-only
Library mode: register a token source before serving:
```go
srv := proxy.NewServer(cfg, hook)
srv.SetTokenSource("vertex", proxy.StaticTokenSource{AccessToken: accessToken})
// or CachingTokenSource wrapping your refresh function
http.ListenAndServe(cfg.Listen, srv.Handler())
Air-gapped tests use fakes only; production ADC/SA JWT exchange is your injector’s job (optional Google auth libraries outside this module).
### OpenAI org / project headers
On **OpenAI-family** egress only (`openai`, `openai_compat`), the gateway forwards:
| Request header | Forwarded? |
| `OpenAI-Organization` | yes |
| `OpenAI-Project` | yes |
| `OpenAI-Beta` | yes (Realtime / beta surfaces) |
These are **not** sent to Anthropic or Google kinds. Still forwarded when `api_key_env` replaces the API key (org/project are orthogonal to the secret).
Usage events include `key_hash`: first 12 hex chars of SHA-256 of the forwarded credential — correlate without storing secrets.

### Rate-limit & correlation header policy

Response headers are **allowlisted** (not full copy). Hop-by-hop and `Set-Cookie` are never relayed.

| Direction | Headers |
|---|---|
| **Upstream → client** | `Content-Type`, `Content-Length`, `Content-Encoding`, `Retry-After`, `X-Request-Id` / `Request-Id`, `x-ratelimit-*`, `anthropic-ratelimit-*`, `x-goog-*` (rate/quota style), `Openai-Processing-Ms`, `Openai-Version`, `Openai-Organization` |
| **Gateway → client** | `X-Gateway-Request-Id` (gateway correlation id; does **not** remove upstream `x-request-id`) |
| **Client → OpenAI-family** | `OpenAI-Organization`, `OpenAI-Project`, `OpenAI-Beta` (plus auth) |
| **Never** | `Set-Cookie`, `Connection`, `Transfer-Encoding`, `Upgrade` (except intentional WS handshake), upstream mid-proxy auth challenges |

Applies to chat, Responses, Files, Moderations, media, Anthropic messages/batches, and Google generateContent passthrough/stream paths.

---

## Passthrough vs translation

### Passthrough (same family)

1. Parse body as generic JSON  
2. Rewrite `model` only  
3. OpenAI streams: inject `stream_options.include_usage` if unset  
4. Forward bytes; scan SSE for usage  
5. Upstream ≥400 relayed largely as-is  

Full fidelity (e.g. Claude Code → Anthropic).

### Translation (cross family)

Parse → **canonical** (Anthropic-shaped blocks) → build upstream wire → parse response/stream → serialize caller dialect.

| Feature | Translated |
|---|---|
| Text, system / developer | yes |
| Images (URL / base64) + OpenAI `image_url.detail` | yes (detail is OpenAI round-trip; other dialects drop detail) |
| OpenAI `input_audio` / `file` content parts | yes → canonical audio/document blocks (OpenAI egress rebuilds) |
| Tools + tool choice (`required` ↔ `any`) | yes |
| OpenAI `parallel_tool_calls` | yes (canonical `ParallelToolCalls`; Anthropic polarity mapping separate) |
| Tool result multimodal content (text + image) | yes (best-effort on Google) |
| Streaming | yes |
| temperature, top_p, stop | yes |
| `max_tokens` / `max_completion_tokens` | yes (source field preserved on OpenAI egress) |
| `frequency_penalty` / `presence_penalty` / `seed` | yes on OpenAI egress; Anthropic omits |
| OpenAI `response_format` (`json_object` / `json_schema`) | yes via canonical `ResponseFormat` (OpenAI↔OpenAI; cross-dialect mapping may land later) |
| OpenAI `reasoning_effort` | yes via canonical `ThinkingConfig` (OpenAI↔OpenAI; cross-dialect effort↔budget best-effort later) |
| Thinking / `reasoning_content` on assistant history | yes (required for DeepSeek/Kimi/Z.AI tool loops) |
| Thinking blocks (Anthropic) | carried when present |
| Usage details (`cached_tokens`, `reasoning_tokens`, Anthropic cache write) | yes when upstream reports them |
| OpenAI `service_tier` / `system_fingerprint` | optional OpenAI-only metadata |
| Google `safetySettings` | yes on Google egress only |
| OpenAI `n` / Google `candidateCount` | **n=1 only** (`n>1` → `bad_request`) |
| Non-function OpenAI tools (`type` ≠ `function`) | **rejected** (`bad_request`) |
| Anthropic `cache_control` | **passthrough-only** (stripped on translate rebuild) |
| OpenAI `logprobs`, `logit_bias`, `top_logprobs`, … | **dropped** (see golden drop list) |

**Caveats**

- Anthropic client → OpenAI stream: `message_start.input_tokens` may be `0` until the final event (OpenAI reports usage late). Hooks still get final counts.
- OpenAI → Anthropic without `max_tokens`: default **4096**.
- Gateway errors use the **caller** dialect envelope; translation path reshapes upstream errors the same way.
- Prompt caching (`cache_control`) is preserved only on **Anthropic→Anthropic passthrough**. Cross-dialect translate rebuilds messages from canonical and drops breakpoints.
- Multi-choice (`n` / `candidateCount` > 1) is not supported on translate; use passthrough to a same-family provider if you need multiple candidates.
- **Reasoning content tool loops:** openai_compat providers (DeepSeek, Kimi/Moonshot, Z.AI, …) often **require** prior assistant `reasoning_content` on subsequent tool-loop turns. Passthrough keeps it after model rewrite; translate-to-OpenAI rebuilds it from `BlockThinking`. Do not strip this field in client middleware.
- **Redacted thinking → OpenAI:** Anthropic `redacted_thinking` maps to `BlockThinking{Redacted:true}`. OpenAI policy is to **omit** it (no opaque `reasoning_content` invented). History that will re-enter Anthropic should stay on Anthropic passthrough when redacted segments must be preserved.
- Structured outputs / reasoning_effort: OpenAI→canonical→OpenAI is lossless for supported shapes; full Anthropic/Google matrix mapping may still be incomplete for some fields.
- Golden drop lists: [`testdata/fixtures/chat_translate/`](testdata/fixtures/chat_translate/).

---

## Hooks & usage events

| Sink | Config | Behavior |
|---|---|---|
| **JSONL** | `hooks.jsonl.output` | One JSON line per chat request |
| **Webhook** | `hooks.webhook.url` | Async POST; failures logged only |
| **Go** | `gateway.WithHook(h)` | In-process after response |

Invariant: **exactly one** `UsageEvent` per proxied chat, media, embeddings, or audio request (including errors and aborts). Not emitted for `count_tokens`, `GET /v1/models`, or `healthz`.
Invariant: **exactly one** `UsageEvent` per `/v1/chat/completions` and `/v1/messages` (including errors and aborts). Not emitted for `count_tokens`, Gemini `:countTokens`, `GET /v1beta/models*`, or `healthz`.
Invariant: **exactly one** `UsageEvent` per proxied chat, media, and embeddings request (including errors and aborts). Not emitted for `count_tokens` / `healthz`.

JSONL/Go must not block the request path. Webhook is non-blocking (background POST).

**Multi-replica:** use `stdout` or webhook — not a shared local file.

### Event schema

```json
{
  "request_id": "req_<16 hex>",
  "time": "RFC3339",
  "dialect_in": "openai | anthropic | google",
  "provider": "configured name",
  "model": "public id from client",
  "upstream_model": "id sent upstream",
  "modality": "text | image_gen | video_gen | audio_speech | audio_transcribe | realtime",
  "transport": "http | websocket",
  "tokens_in": 0,
  "tokens_out": 0,
  "cached_tokens": 0,
  "cache_write_tokens": 0,
  "reasoning_tokens": 0,
  "estimated": false,
  "media": {
    "units": 1,
    "unit_kind": "image | video_second | audio_character | audio_minute | session_minute",
    "duration_ms": 0,
    "size": "1024x1024",
    "format": "b64_json"
  },
  "stream": false,
  "status": "ok | upstream_error | client_abort | bad_request",
  "http_status": 200,
  "latency_ms": 0,
  "ttft_ms": 0,
  "key_hash": "12 hex or empty"
}
```

Empty `modality` / `transport` means legacy text over HTTP. `media` is omitted when unused.

**Media example** (image generations, `n=2`):
```json
{
  "modality": "image_gen",
  "transport": "http",
  "estimated": true,
  "media": { "units": 2, "unit_kind": "image", "size": "1024x1024" },
  "status": "ok",
  "http_status": 200
}
```
Video **create** uses `unit_kind=video_second` with `units` from `seconds`/`duration` when present; video **poll** emits `units=0` (operational). Gateway never multiplies by unit prices.
Optional detail fields (`cached_tokens`, `cache_write_tokens`, `reasoning_tokens`) are omitted when zero. `tokens_out` already includes reasoning tokens when the upstream folds them into completion totals — do not add `reasoning_tokens` again without checking the provider.
| `status` | When |
|---|---|
| `ok` | Success |
| `bad_request` | Client / routing / parse / capability error |
| `upstream_error` | Transport failure, HTTP ≥400, broken stream |
| `client_abort` | Client canceled mid-flight (`http_status` 499 if no response) |

`estimated: true` → upstream reported no usage (tokens not measured).  
Type source: [`hooks/hooks.go`](hooks/hooks.go).

---

## Claude Code

```bash
export ANTHROPIC_BASE_URL=http://localhost:8787
export ANTHROPIC_API_KEY=<key valid for the routed provider>
# optional: export ANTHROPIC_MODEL=deepseek/deepseek-chat
claude
```

Or [`examples/claude-code.sh`](examples/claude-code.sh). Anthropic upstream = byte passthrough; OpenAI-compat = full translation (see stream token caveat).

Release / regression checklist: [docs/claude-code-checklist.md](docs/claude-code-checklist.md).

---

## Library use

```go
package main

import (
	"context"
	"net/http"

	gateway "github.com/inja-online/llm-gateway"
	"github.com/inja-online/llm-gateway/config"
	"github.com/inja-online/llm-gateway/hooks"
)

func main() {
	cfg, err := config.Load("gateway.yaml")
	if err != nil {
		panic(err)
	}
	h, err := gateway.New(cfg, gateway.WithHook(hooks.Func(func(ctx context.Context, ev hooks.UsageEvent) {
		// billing / analytics — do not block
	})))
	if err != nil {
		panic(err)
	}
	http.ListenAndServe(cfg.Listen, h)
}
```

YAML hooks (JSONL, webhook) wire automatically; `WithHook` adds more.  
Binary: [`cmd/gateway`](cmd/gateway).

---

## Architecture

```
cmd/gateway/                     binary (graceful shutdown, env overrides)
gateway.go                       library New(cfg, opts...) → http.Handler

proxy/                           route → forward → meter
canonical/                       dialect-neutral types (Anthropic-shaped superset)
ingress/{openai,anthropic,google}/   client dialects (Gemini OpenAI-compat = openai)
egress/{openai,anthropic,google}/    upstream adapters (Gemini OpenAI-compat = openai)
hooks/                           UsageEvent; jsonl + webhook sinks
config/                          YAML + GATEWAY_* env
internal/sse/                    SSE scan helpers
deploy/k8s/                      Kubernetes manifests
```

| Path | Flow |
|---|---|
| Cross-dialect | `ingress.Parse` → canonical → `egress.Build` → upstream → parse/stream → `ingress.Serialize` |
| Same dialect | rewrite `model` (+ stream usage / path) → forward bytes |
| Gemini OpenAI-compat | OpenAI dialect + `openai_compat` provider (passthrough) |

---

## Deploy

| Artifact | Use |
|---|---|
| [`Dockerfile`](Dockerfile) | Multi-stage **distroless** image, non-root |
| [`docker-compose.yml`](docker-compose.yml) | Local / Docker Desktop |
| [`deploy/k8s/gateway.yaml`](deploy/k8s/gateway.yaml) | Namespace, ConfigMap, Deployment (×2), Service, probes |
| [`gateway.example.yaml`](gateway.example.yaml) | Sample config |

Same binary and config on a Windows laptop, a Linux VM, or a cluster. No separate “cloud mode.”

---

## Development & CI

```bash
go test ./...
go test -race ./...
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1
go vet ./...
docker build -t llm-gateway:dev .
```

**CI** ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) on push/PR:

- build, vet, `go test -race`
- coverage gate **≥ 90%**
- binary smoke (`/healthz`)
- Docker image build + container healthz

**Release** ([`.github/workflows/release.yml`](.github/workflows/release.yml)) on `v*` tags:

- multi-arch binaries + checksums → GitHub Release

```bash
git tag v0.1.0 && git push origin v0.1.0
```

---

## Roadmap

- [x] Google / Gemini native dialect + egress (`kind: google`) and OpenAI-compat base
- [x] Image + video generation passthrough (`/v1/images/*`, `/v1/videos`)
- [x] `GET /v1/models` (+ `GET /v1/models/{id}`) from config/aliases
- [x] Embeddings (`POST /v1/embeddings`, Gemini `:embedContent` / `:batchEmbedContents`)
- [x] Optional edge auth (`edge_auth`)
- [x] Vertex / ADC **TokenSource** helper (interface + fakes; real ADC injectable)
- [x] OpenAI Responses + Files + Moderations passthrough
- [x] Anthropic Message Batches proxy (`/v1/messages/batches*`)
- [x] Rate-limit / OpenAI org-project header policy
- [x] Realtime WS skeleton (OpenAI + Google Live) + session limits
- [x] Conversations API decision: **stub 501** (not full support)
- [x] Realtime ↔ Live **bridge deferred** (fail-closed `unsupported_realtime_bridge`; same-protocol passthrough only)
- [ ] Production TLS `wss` dial for realtime upstreams
- [ ] Optional Realtime ↔ Live IR bridge (future milestone; not M3)
- [ ] Cross-dialect image/video generation translation
- [x] HTTP audio (TTS/STT): OpenAI + Anthropic-gateway + Google `:generateSpeech`

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (includes **adding a modality** guide).  
Security: [SECURITY.md](SECURITY.md). Changelog: [CHANGELOG.md](CHANGELOG.md).  
Matrix: [docs/compatibility-matrix.md](docs/compatibility-matrix.md). Field drops: [docs/deprecation-policy.md](docs/deprecation-policy.md).

---

## License

[MIT](LICENSE) © 2026 [inja-online](https://github.com/inja-online)
