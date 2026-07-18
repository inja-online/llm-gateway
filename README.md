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

Clients speak **OpenAI**, **Anthropic**, or **Gemini (native)**. The gateway routes to any upstream (OpenAI, Anthropic, Google, DeepSeek, xAI, Moonshot, OpenRouter, vLLM, …), translates dialects when needed, and emits **exactly one usage event per chat request** — no database, no auth layer.

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
| Google | `openai` / `openai_compat` / `anthropic` | **translated** |

Also shipped:

- Streaming + non-streaming, tools, multimodal **input** images, system prompts
- OpenAI-compatible **image generation** (`/v1/images/*`) and **video generation** (`/v1/videos`)
- OpenAI **Responses** API (`/v1/responses`, streaming SSE, GET/DELETE by id)
- OpenAI **Files** API proxy (`/v1/files*`) and **Moderations** (`/v1/moderations`)
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
| **Native** | `generateContent` / `streamGenerateContent` | **Yes** — dedicated dialect `POST /v1beta/models/{model}:…` (`ingress/google`) | **Yes** — `kind: google` (`egress/google`), `x-goog-api-key` |
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

**Anthropic SDK** (including non-Anthropic models — gateway translates):

```python
from anthropic import Anthropic
client = Anthropic(base_url="http://localhost:8787", api_key="<key for target provider>")
r = client.messages.create(
    model="deepseek/deepseek-chat",
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
| `POST` | `/v1beta/models/{model}:generateContent` | Gemini **native** dialect |
| `POST` | `/v1beta/models/{model}:streamGenerateContent` | Gemini native streaming (upstream `?alt=sse`) |
| `POST` | `/v1/responses` | OpenAI Responses create (passthrough; `stream:true` SSE) |
| `GET` | `/v1/responses/{id}` | Retrieve stored response (proxy only; no gateway storage) |
| `DELETE` | `/v1/responses/{id}` | Delete stored response upstream |
| `POST` | `/v1/files` | Upload file (multipart passthrough) |
| `GET` | `/v1/files` | List files |
| `GET` | `/v1/files/{id}` | Retrieve file metadata |
| `DELETE` | `/v1/files/{id}` | Delete file upstream |
| `GET` | `/v1/files/{id}/content` | Download file content (streamed) |
| `POST` | `/v1/moderations` | OpenAI Moderations passthrough |
| `POST` | `/v1/images/generations` | Image generation (OpenAI-compat passthrough) |
| `POST` | `/v1/images/edits` | Image edits (JSON or multipart passthrough) |
| `POST` | `/v1/images/variations` | Image variations (passthrough) |
| `POST` | `/v1/videos` | Video generation job create (OpenAI / Gemini OpenAI-compat) |
| `GET` | `/v1/videos/{id}` | Video job status (`?provider=` or `defaults.openai_dialect`) |
| `GET` | `/v1/realtime` | OpenAI Realtime WebSocket upgrade (skeleton passthrough) |
| `GET` | `/v1beta/models/{model}:bidiGenerateContent` | Google Live WebSocket (skeleton) |
| `POST` | `/v1/messages/count_tokens` | Token count (proxy or estimate) |
| `GET` | `/healthz` | Liveness / readiness: `{"status":"ok"}` |

There is **no** separate `/v1beta/openai/…` ingress: Gemini’s OpenAI-compat API is the same Chat Completions shape, so clients use `/v1/chat/completions` and a provider such as `google_openai`.

**Not exposed:** OpenAI Conversations / Assistants threads — prefer **Responses** + client-side state (or Files). The gateway stays stateless.

### Responses API

OpenAI-family only (`kind: openai` or `openai_compat`). Same model routing as chat (`aliases` / `provider/model` / `defaults.openai_dialect`).

| Call | Notes |
|---|---|
| `POST /v1/responses` | Rewrites `model`; preserves unknown JSON fields; one usage event (`input_tokens` / `output_tokens` when present) |
| `stream: true` | Byte-faithful SSE of typed events (`response.created`, `response.completed`, …); usage from completed event |
| `GET` / `DELETE /v1/responses/{id}` | Provider: `?provider=` **or** `X-Provider` **or** `defaults.openai_dialect` — gateway does **not** store bodies |

```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key="sk-...")
r = client.responses.create(model="openai/gpt-4o", input="hello")
```

### Files API

OpenAI-family proxy. **Files live on the upstream provider**, not on the gateway (no disk spool beyond the in-flight request body; global body limit **32 MiB**).

Provider selection (no model field): `?provider=` → `X-Provider` → `defaults.openai_dialect`.

Usage: one operational event per call (`estimated: true`, zero tokens).

### Moderations

`POST /v1/moderations` — OpenAI-family only; rewrites `model` when present; otherwise uses default OpenAI-family provider.

### Realtime (WebSocket, skeleton)

| Path | Provider | Capability |
|---|---|---|
| `GET /v1/realtime?model=…` | `openai` / `openai_compat` with `capabilities.realtime` | OpenAI Realtime |
| `GET /v1beta/models/{model}:bidiGenerateContent` | `kind: google` with `realtime` | Google Live |

Process limits from config (`realtime.max_sessions`, `realtime.max_session_minutes`). One usage event on session end (`modality=realtime`, `transport=websocket`, `media.unit_kind=session_minute`).

**Gaps (TODO):** production TLS/`wss` dial, full protocol edge cases, OpenAI Realtime ↔ Google Live bridge. Hermetic tests cover upgrade + limits + capability deny.

### Image & video generation

Chat multimodal **inputs** (image URL / base64 in messages) are already part of chat translation. **Generation** APIs are separate OpenAI-compatible routes:

| API | Providers | Notes |
|---|---|---|
| Images | `openai`, `openai_compat` (incl. `google_openai`) | Rewrite `model`; emit one usage event (`estimated: true` if no tokens) |
| Videos | same | Create is `POST` with `model`; poll with `GET /v1/videos/{id}?provider=google_openai` |
| Native Gemini image-in-chat | `kind: google` | Via `generateContent` image models / modalities on the Google dialect |

Not supported (yet): cross-dialect translation of image/video generation (e.g. OpenAI images → Anthropic). Route these to an OpenAI-family provider only.

```python
# Image gen via Gemini OpenAI-compat
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key=os.environ["GEMINI_API_KEY"])
img = client.images.generate(model="google_openai/gemini-2.5-flash-image", prompt="a sheepadoodle in a cape")

# Video job (poll status)
# POST /v1/videos  {"model":"google_openai/veo-3.1-generate-preview","prompt":"..."}
# GET  /v1/videos/{id}?provider=google_openai
```

No `/v1/models` yet. Unknown routes → standard 404.

**Limits**

| Limit | Value |
|---|---|
| Request / response body | 32 MiB |
| Overall proxy timeout | none (streams may be long-lived) |
| Upstream response header wait | 60s |
| `count_tokens` → Anthropic | 15s |
| Shutdown drain | 30s |

Client disconnect cancels the upstream context.

### `count_tokens`

| Resolved provider | Behavior |
|---|---|
| `anthropic` | Proxy to real `…/messages/count_tokens`; fall back to estimate on failure |
| other | Local estimate only (~1 token / 4 chars of text & tool schema) |

Estimate is for client compatibility (e.g. Claude Code), **not billing**. No usage event is emitted.

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
| `defaults.openai_dialect` | no | Provider for bare models on OpenAI ingress |
| `defaults.anthropic_dialect` | no | Provider for bare models on Anthropic ingress |
| `defaults.google_dialect` | no | Provider for bare models on Gemini ingress |
| `aliases` | no | Public id → `provider/upstream-model` |
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

The gateway **does not authenticate callers**. It reads:

1. `Authorization: Bearer <key>`, or
2. `x-api-key: <key>`, or
3. `x-goog-api-key: <key>`

…and forwards that credential using the provider’s auth scheme. **The key must be valid for the target provider.** An OpenAI key routed to `anthropic/...` will be rejected upstream.

With `api_key_env` set and the env non-empty, the **server-held key replaces** the client key (clients can send a dummy).

### OpenAI org / project headers

On **OpenAI-family** egress only (`openai`, `openai_compat`), the gateway forwards:

| Request header | Forwarded? |
|---|---|
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

Applies to chat, Responses, Files, Moderations, media, Anthropic messages, and Google generateContent passthrough/stream paths.

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
| Images (URL / base64) | yes |
| Tools + tool choice (`required` ↔ `any`) | yes |
| Streaming | yes |
| temperature, top_p, stop | yes |
| `max_tokens` / `max_completion_tokens` | yes |
| Thinking blocks (Anthropic) | carried when present |
| OpenAI `n`, `logprobs`, `response_format`, `seed`, … | **dropped** |

**Caveats**

- Anthropic client → OpenAI stream: `message_start.input_tokens` may be `0` until the final event (OpenAI reports usage late). Hooks still get final counts.
- OpenAI → Anthropic without `max_tokens`: default **4096**.
- Gateway errors use the **caller** dialect envelope; translation path reshapes upstream errors the same way.

---

## Hooks & usage events

| Sink | Config | Behavior |
|---|---|---|
| **JSONL** | `hooks.jsonl.output` | One JSON line per chat request |
| **Webhook** | `hooks.webhook.url` | Async POST; failures logged only |
| **Go** | `gateway.WithHook(h)` | In-process after response |

Invariant: **exactly one** `UsageEvent` per `/v1/chat/completions` and `/v1/messages` (including errors and aborts). Not emitted for `count_tokens` / `healthz`.

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
  "tokens_in": 0,
  "tokens_out": 0,
  "estimated": false,
  "stream": false,
  "status": "ok | upstream_error | client_abort | bad_request",
  "http_status": 200,
  "latency_ms": 0,
  "ttft_ms": 0,
  "key_hash": "12 hex or empty"
}
```

| `status` | When |
|---|---|
| `ok` | Success |
| `bad_request` | Client / routing / parse error |
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
- [x] OpenAI Responses + Files + Moderations passthrough
- [x] Rate-limit / OpenAI org-project header policy
- [x] Realtime WS skeleton (OpenAI + Google Live) + session limits
- [ ] Production TLS `wss` dial + Realtime ↔ Live bridge
- [ ] `GET /v1/models`
- [ ] Optional request auth at the gateway edge
- [ ] Vertex AI (ADC / service-account) auth helper
- [ ] Cross-dialect image/video generation translation

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

---

## License

[MIT](LICENSE) © 2026 [inja-online](https://github.com/inja-online)
