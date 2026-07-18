# Multimodal Multi-Dialect LLM Gateway — Design Spec

**Status:** Draft for approval (implementation not started)  
**Date:** 2026-07-18  
**Module:** `github.com/inja-online/llm-gateway`  
**Related:** current chat triple-dialect + OpenAI-compat image/video passthrough

---

## 1. Goal

Extend the gateway from a **text-first** multi-dialect proxy into a **general multimodal gateway** that:

1. Accepts **three client dialects** (`openai`, `anthropic`, `google`) for **text, image generation, video generation, and voice (TTS/STT)**.
2. Routes to **any capable upstream provider**, with **bidirectional translation** when ingress dialect ≠ egress wire.
3. Supports **realtime** over WebSockets using **real protocols only** (OpenAI Realtime, Google Live), with a **protocol bridge** between them — **no invented Anthropic WebSocket dialect**.
4. Remains a **single static binary**, **stateless**, **YAML-config**, **one usage event per request unit**, **air-gapped-testable**, and developed **TDD-first**.

Non-goals for this design:

- In-gateway currency pricing / rate cards.
- Gateway-level end-user auth product (optional edge auth may land later; not required here).
- Inventing a fake Anthropic realtime protocol.
- Guaranteeing lossless parity for every vendor-specific media field.

---

## 2. Product decisions (locked)

| ID | Decision | Choice |
|----|----------|--------|
| D1 | Client ambition | **Full multi-dialect surfaces** for every **HTTP** modality (OpenAI + Anthropic-shaped + Google-shaped). |
| D2 | Missing vendor APIs | **Hybrid:** invent **stable gateway HTTP media contracts** for Anthropic (and Google where no public twin exists). Realtime only for **real** protocols. |
| D3 | Translation | **Both ways** where capability exists: any HTTP ingress dialect → any capable egress; realtime OpenAI Realtime ↔ Google Live. |
| D4 | Architecture | **Capability-centric multimodal core** (not ad-hoc routes bolted forever). |
| D5 | Testing | **TDD for all work**; **fully air-gapped** tests (no public internet, no real provider keys); hermetic fakes/fixtures only. |
| D6 | Dependencies | Prefer **stdlib-first**; new deps only with explicit justification (WebSocket may add one small focused library if stdlib framing is too costly). Runtime still aims for minimal surface (`yaml.v3` + optional WS helper). |
| D7 | Pricing | Gateway emits **tokens + media units**; **consumers** compute price. |

---

## 3. Current baseline

| Area | Today |
|------|--------|
| Text | OpenAI ↔ Anthropic ↔ Google; passthrough + canonical translation |
| Image/video **gen** | OpenAI-compat **passthrough only** (`/v1/images/*`, `/v1/videos`) |
| Voice | None |
| Realtime WS | None |
| Canonical | Chat blocks: text, image **input**, tools, thinking |
| Tests | `httptest` fake upstreams; coverage gate ≥90%; no live provider CI |
| Deps | `gopkg.in/yaml.v3` only |

This design **extends** that model; it does not replace chat behavior.

---

## 4. Core concepts

### 4.1 Dialect

Wire/contract family the **client** speaks: `openai` | `anthropic` | `google`.

### 4.2 Modality

| Modality | Description |
|----------|-------------|
| `text` | Chat / messages / generateContent |
| `image_gen` | Image generation / edit / variation |
| `video_gen` | Video create / poll / optional content fetch |
| `audio_speech` | Text-to-speech |
| `audio_transcribe` | Speech-to-text (+ optional translation) |
| `realtime` | Bidirectional live session |

### 4.3 Transport

`http` | `websocket`.

### 4.4 Provider kind (egress)

`openai` | `openai_compat` | `anthropic` | `google` (extensible).

### 4.5 Capability

Whether a provider can serve a modality:

- `native` — same-family wire, passthrough (model rewrite + auth + metering)
- `translated` — via canonical
- `unsupported` — fail closed before upstream, dialect-shaped error

### 4.6 Canonical

Dialect-neutral internal model **per modality**. Chat keeps Anthropic-shaped blocks; media/realtime get their own records. Translation only on cross-family paths; same-family uses passthrough.

### 4.7 Usage unit

Exactly **one** `UsageEvent` per:

- HTTP request that enters a modality handler (success, client error, upstream error, abort), or
- Realtime **session end** (plus optional future mid-session samples — not required for v1).

No usage event for `/healthz` or pure capability discovery if added later without proxying.

---

## 5. Public HTTP API (end-state)

### 5.1 Text (existing)

| Dialect | Methods |
|---------|---------|
| OpenAI | `POST /v1/chat/completions` |
| Anthropic | `POST /v1/messages`, `POST /v1/messages/count_tokens` |
| Google | `POST /v1beta/models/{model}:generateContent`, `:streamGenerateContent` |

### 5.2 Image generation

| Dialect | Routes | Notes |
|---------|--------|-------|
| OpenAI | `POST /v1/images/generations`, `/edits`, `/variations` | OpenAI Images wire |
| Anthropic (gateway-defined) | `POST /v1/images`, `POST /v1/images/edits` | Requires `anthropic-version`; Anthropic error envelope; **not** `/v1/images/generations` |
| Google | `POST /v1beta/models/{model}:generateImages` | Google-shaped request/response/errors; model in path |

**Path disambiguation**

| Client base URL (typical) | Image generate path |
|---------------------------|---------------------|
| OpenAI SDK `…/v1` | `/images/generations` |
| Anthropic SDK `…` (host root) | `/v1/images` + `anthropic-version` |
| Google native | `/v1beta/models/{model}:generateImages` |

Rules:

- OpenAI-only paths never accept Anthropic media schema.
- If `anthropic-version` is sent to `/v1/images/generations` → `400` with clear “use `POST /v1/images` for Anthropic dialect media.”
- If Anthropic media path lacks `anthropic-version` → `400` (same spirit as Anthropic Messages).

### 5.3 Video generation

| Dialect | Routes |
|---------|--------|
| OpenAI | `POST /v1/videos`, `GET /v1/videos/{id}` |
| Anthropic (gateway) | `POST /v1/videos`, `GET /v1/videos/{id}` + `anthropic-version` |
| Google | `POST /v1beta/models/{model}:generateVideos`, `GET /v1beta/videos/{name}` (operation poll) |

Canonical job model maps provider job/operation IDs. Poll must not require the client to know upstream id format beyond what the create response returned in that dialect.

### 5.4 Voice (HTTP)

| Dialect | TTS | STT |
|---------|-----|-----|
| OpenAI | `POST /v1/audio/speech` | `POST /v1/audio/transcriptions`, `/translations` |
| Anthropic (gateway) | `POST /v1/audio/speech` | `POST /v1/audio/transcriptions` (+ optional translations) |
| Google | `POST /v1beta/models/{model}:generateSpeech` | `POST /v1beta/models/{model}:transcribe` |

STT accepts multipart and, where schema allows, JSON+base64. Binary audio responses (TTS) preserve `Content-Type` and raw body on passthrough; translation path may re-encode only when required by dialect contract.

### 5.5 Realtime (WebSocket)

| Ingress | Route | Egress support |
|---------|-------|----------------|
| OpenAI Realtime | `GET` + Upgrade `/v1/realtime` | Passthrough → OpenAI / capable `openai_compat`; bridge → Google Live |
| Google Live / Bidi | Documented `/v1beta/...` live path | Passthrough → Google; bridge → OpenAI Realtime |
| Anthropic WS | **Not provided** | Clients use OpenAI Realtime ingress if they need live; Anthropic upstream realtime = unsupported |

---

## 6. Capability matrix (routing truth)

Defaults (overridable in YAML for `openai_compat`):

| Kind | text | image_gen | video_gen | audio_* | realtime |
|------|------|-----------|-----------|---------|----------|
| `openai` | native | native | native | native | native |
| `openai_compat` | native | **opt-in** | **opt-in** | **opt-in** | **opt-in** |
| `anthropic` | native | unsupported | unsupported | unsupported | unsupported |
| `google` | native | native/translated | native/translated | native/translated | Live native |

**Fail closed:** resolving `anthropic/…` for `image_gen` returns dialect-shaped `unsupported_provider_capability` without calling upstream.

**openai_compat default:** text only unless `capabilities` explicitly enable media/realtime. Prevents silent routing to hosts that 404.

Example:

```yaml
providers:
  google_openai:
    kind: openai_compat
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
    capabilities:
      text: true
      image_gen: true
      video_gen: true
      audio_speech: false
      audio_transcribe: false
      realtime: false
```

---

## 7. Architecture

```
 Clients (3 dialects, HTTP + WS)
              │
              ▼
 ┌────────────────────────────┐
 │ Ingress (parse, dialect)   │
 └─────────────┬──────────────┘
               ▼
 ┌────────────────────────────┐
 │ Router + Capability check  │
 └─────────────┬──────────────┘
       ┌───────┼────────┐
       ▼       ▼        ▼
 Passthrough Translate  Realtime bridge
       │       │        │
       └───────┼────────┘
               ▼
 ┌────────────────────────────┐
 │ Egress adapters            │
 │ openai / compat / anthropic / google │
 └─────────────┬──────────────┘
               ▼
 ┌────────────────────────────┐
 │ Hooks / UsageEvent         │
 └────────────────────────────┘
```

### 7.1 Package layout (target)

```
canonical/
  chat.go          # existing + BlockAudio input
  image.go
  video.go
  audio.go
  realtime.go      # bridge intermediate representation (IR)
  capability.go    # modality constants shared types if needed
ingress/
  openai/          # chat + images + videos + audio + realtime upgrade helper
  anthropic/       # chat + gateway images/videos/audio
  google/          # generateContent + images/videos/audio + live
egress/
  openai/
  anthropic/
  google/
proxy/
  router.go        # model resolve + capability
  server.go        # route table
  chat_*.go
  media_*.go       # generalized image/video (replace passthrough-only)
  audio_*.go
  realtime/        # accept, dial, passthrough, bridge, limits
hooks/
  hooks.go         # extended UsageEvent
testdata/
  fixtures/        # golden HTTP/WS frames (air-gapped)
  README.md        # how fixtures are produced offline
internal/testutil/ # shared fake upstreams, WS test helpers, collectors
```

### 7.2 Request pipeline (HTTP)

1. Match route → dialect + modality.
2. Read body (respect 32 MiB limit; multipart as raw when needed).
3. Resolve model → `Route{Provider, UpstreamModel}`.
4. Capability check.
5. If ingress family matches egress family for that modality → **passthrough** (rewrite model/auth only).
6. Else parse → canonical → build egress wire → send → parse response → serialize ingress dialect.
7. Emit one `UsageEvent`.

### 7.3 Realtime pipeline

1. HTTP Upgrade; validate auth headers (forward/replace via `api_key_env` same as HTTP).
2. Resolve model/session config from query/first events per protocol rules.
3. Capability check for `realtime`.
4. Same protocol → byte/event passthrough with auth rewrite.
5. Cross protocol → run **bridge** (IR mapping, audio format conversion best-effort, tools best-effort).
6. On session end → usage event (`media.unit_kind=session_minute` / audio seconds when known).

---

## 8. Canonical models (normative sketch)

### 8.1 Chat extensions

Add `BlockAudio` for **input** audio in multimodal chat:

- `Kind`: `base64` | `url`
- `MediaType`, `Data`
- Optional `Transcript` if client supplied

### 8.2 ImageGenRequest / Response

Request: model, prompt, n, size, quality, style, response_format (`url`|`b64`), mode (`generate`|`edit`|`variation`), source images, mask, seed, `Extra map[string]json.RawMessage` for passthrough-only hints.

Response: id, model, images[{b64|url, media_type, revised_prompt}], usage if any.

### 8.3 VideoGenRequest / Response

Request: model, prompt, duration, resolution, aspect, references, operation (`create`|`get`), job_id.

Response: job id, status (`queued`|`processing`|`completed`|`failed`), progress, result URLs/b64, error message, usage if any.

### 8.4 AudioSpeechRequest / Response

Request: model, text, voice, format, speed.

Response: raw audio bytes + content-type **or** dialect JSON wrapper if that dialect requires it (OpenAI speech is raw audio by default).

### 8.5 AudioTranscribeRequest / Response

Request: model, audio, language, prompt, translate bool, response_format.

Response: text (+ segments if dialect supports and upstream provided).

### 8.6 Realtime IR

Session config: model, voice, modalities, tools, turn detection, output format.

Events (non-exhaustive): `session.update`, `input_audio.append`, `input_audio.commit`, `response.create`, `response.text.delta`, `response.audio.delta`, `response.function_call`, `error`, `session.end`.

Bridge maps OpenAI Realtime event names ↔ Google Live message types; undocumented fields go to `Extra` or are dropped with metrics in tests (explicit drop lists).

---

## 9. Gateway-defined media contracts

### 9.1 Principles

1. **Stable & versioned** — document “Gateway Media Contract v1”; Anthropic routes still require `anthropic-version`.
2. **Feel native** — Anthropic snake_case + error shape; Google model-in-path + `x-goog-api-key` + Google errors.
3. **Round-trip friendly** — field sets chosen so OpenAI ↔ Anthropic-gateway ↔ Google-gateway map without inventing dead fields.
4. **Published** — README tables + this spec are the contract; golden fixtures lock them.

### 9.2 Anthropic gateway image generate (v1)

Request:

```json
{
  "model": "google/imagen-3",
  "prompt": "a red cube",
  "n": 1,
  "size": "1024x1024",
  "response_format": "base64"
}
```

Response:

```json
{
  "id": "img_01",
  "type": "image_generation",
  "model": "imagen-3",
  "data": [{ "b64_json": "...", "media_type": "image/png" }],
  "usage": { "input_tokens": 0, "output_tokens": 0 }
}
```

(Full field tables finalized in implementation PR0 docs; fixtures are authoritative.)

### 9.3 Errors (all modalities)

Ingress dialect envelope always.

Gateway logical codes (mapped into each dialect):

| Code | When |
|------|------|
| `unsupported_modality` | Route exists but modality disabled globally (future) |
| `unsupported_provider_capability` | Provider cannot serve modality |
| `unsupported_realtime_bridge` | WS pair has no bridge |
| `invalid_media_request` | Schema/parse failure |
| `upstream_error` | Transport / HTTP ≥400 / WS failure |

---

## 10. Routing & config

### 10.1 Model resolution (all modalities)

1. `aliases` exact match  
2. `provider/model` prefix  
3. Bare id → dialect default (`defaults.openai_dialect` | `anthropic_dialect` | `google_dialect`)  
4. Capability check  

Optional later: per-modality default providers — **out of scope unless bare-id conflicts force it**. Prefer aliases.

### 10.2 Config end-state

```yaml
listen: ":8787"
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
  anthropic:
    kind: anthropic
    base_url: "https://api.anthropic.com/v1"
  google:
    kind: google
    base_url: "https://generativelanguage.googleapis.com/v1beta"
  google_openai:
    kind: openai_compat
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
    capabilities:
      text: true
      image_gen: true
      video_gen: true

defaults:
  openai_dialect: openai
  anthropic_dialect: anthropic
  google_dialect: google

realtime:
  max_sessions: 1024
  max_session_minutes: 60

hooks:
  jsonl:
    output: stdout
```

Auth unchanged: forward client key or replace via `api_key_env`; `key_hash` on events.

---

## 11. Usage events (billing hooks)

Backward-compatible extension of `hooks.UsageEvent`:

```json
{
  "request_id": "req_...",
  "time": "RFC3339",
  "dialect_in": "openai|anthropic|google",
  "provider": "name",
  "model": "public",
  "upstream_model": "upstream",
  "modality": "text|image_gen|video_gen|audio_speech|audio_transcribe|realtime",
  "transport": "http|websocket",
  "tokens_in": 0,
  "tokens_out": 0,
  "estimated": true,
  "media": {
    "units": 1,
    "unit_kind": "image|video_second|audio_character|audio_minute|session_minute",
    "duration_ms": 0,
    "size": "1024x1024",
    "format": "mp3"
  },
  "stream": false,
  "status": "ok|upstream_error|client_abort|bad_request",
  "http_status": 200,
  "latency_ms": 0,
  "ttft_ms": 0,
  "key_hash": "12hex"
}
```

Rules:

1. Copy upstream token usage when present → `estimated=false`.
2. Else set `estimated=true` for tokens; still fill `media` from request + success (n, duration, char count, format).
3. Gateway **never** multiplies by unit prices.
4. Video poll may emit usage with `units=0` and status ok (meter create more heavily than poll — document: **create bills media units; poll is operational** with tokens 0 estimated).

---

## 12. Testing strategy (TDD + air-gapped) — normative

This section is as binding as the API surface. Implementation PRs that add behavior without tests **do not meet this design**.

### 12.1 Principles

| Principle | Requirement |
|-----------|-------------|
| **TDD** | For each behavior: **failing test first** → implement → green → refactor. Applies to config, router capability, ingress parse/serialize, egress build/parse, proxy pipelines, WS bridge, hooks. |
| **Air-gapped** | Default `go test ./...` **never** opens connections to the public internet or real provider hosts. No `api.openai.com`, no Gemini, no live keys. |
| **Hermetic** | Tests use `httptest`, in-memory pipes, fake WS peers, and **checked-in fixtures** under `testdata/`. |
| **Deterministic** | No `time.Sleep` for synchronization; use channels, `httptest`, or controllable clocks. No wall-clock flakiness. |
| **No secret material** | CI and unit tests use dummy keys (`sk-test`, `test-key`). Real keys only in local manual scripts (optional, not CI). |
| **Coverage** | Keep **≥90%** package coverage gate; new packages must not be excluded without RFC in this doc. |
| **Race** | `go test -race ./...` remains required in CI. |
| **Offline CI** | CI runners may be network-restricted; tests must pass with network disabled (except localhost `httptest`). |

### 12.2 Test pyramid

```
┌─────────────────────────────────────────────┐
│ E2E / smoke (local binary + httptest upstream)│  few
├─────────────────────────────────────────────┤
│ Proxy integration (dialect × modality × kind)│  matrix
├─────────────────────────────────────────────┤
│ Ingress/egress unit (parse/build/stream)     │  many
├─────────────────────────────────────────────┤
│ pure helpers (router, capability, estimate)  │  many
└─────────────────────────────────────────────┘
```

### 12.3 TDD workflow per PR

1. **Spec slice** — list behaviors from this doc for the PR.
2. **Red** — add table-driven tests that encode the contract (including unsupported cells).
3. **Green** — implement minimum code.
4. **Refactor** — keep tests green.
5. **Air-gap check** — run tests with network blocked when feasible:

   ```bash
   # example local guard (implementation may wrap in make target)
   go test ./... -count=1
   ```

6. **No fixture from live APIs in CI** — if a developer captures a fixture, they sanitize and commit; CI only reads fixtures.

### 12.4 Fixture policy (`testdata/fixtures/`)

| Kind | Content | Rules |
|------|---------|-------|
| HTTP request/response JSON | OpenAI / Anthropic-gateway / Google media samples | No PII; tiny base64 blobs (`YQ==`) |
| Multipart STT | Minimal WAV/PCM bytes generated in test or tiny file | Generated in-test preferred over binary blobs |
| SSE chat streams | Existing style | Keep short |
| Realtime | JSON event sequences for both protocols | Golden in/out for bridge |
| Errors | Upstream 4xx/5xx bodies per dialect | |

`testdata/fixtures/README.md` documents naming: `{dialect}_{modality}_{case}.json`.

**Regeneration:** optional `go test -run TestUpdateFixtures` behind build tag `fixtures` **disabled by default**, and **must not** hit network — only reformats local expected outputs.

### 12.5 Fake upstreams (`internal/testutil`)

Shared helpers (TDD-built):

- `FakeHTTPUpstream` — records path, headers, body; returns scripted status/body/sequence.
- `FakeWSUpstream` — accepts WS, scripts event reads/writes, records client frames.
- `UsageCollector` — existing collector pattern generalized.
- `ConfigFromUpstream(t, kind, url)` — YAML snippets pointing at `httptest` URL.
- `Matrix` helper — iterate dialect × modality × provider kind.

All fakes listen on `127.0.0.1` via `httptest` only.

### 12.6 Required matrix tests

For each HTTP modality (`image_gen`, `video_gen`, `audio_speech`, `audio_transcribe`):

| dialect_in | provider kind | expect |
|------------|---------------|--------|
| openai | openai / openai_compat (cap on) | 200 passthrough or translate |
| openai | google | translate or native path |
| openai | anthropic | **4xx capability** (media) |
| anthropic | openai / google | translate |
| anthropic | anthropic | **4xx capability** (media) |
| google | google | passthrough |
| google | openai | translate |
| * | openai_compat cap off | **4xx capability** |

Chat matrix remains; extend with audio-input block cases.

Realtime:

| ingress | egress | expect |
|---------|--------|--------|
| OpenAI RT | OpenAI RT | passthrough |
| OpenAI RT | Google Live | bridge |
| Google Live | Google Live | passthrough |
| Google Live | OpenAI RT | bridge |
| any | anthropic | capability error before upgrade when possible |

### 12.7 WebSocket testing (air-gapped)

- Use `httptest.NewServer` + `http.Hijacker` or x/net/websocket / nhooyr/gorilla **only in tests** as client against gateway.
- Prefer one production WS implementation; tests drive it via real Upgrade on loopback.
- Bridge tests: **golden event sequences** — given OpenAI client frames, assert Google egress frames (and reverse).
- Simulate client abort mid-session → `status=client_abort`, single usage event.
- No wall-clock “wait for audio”; advance by writing next frame.

### 12.8 Multipart / binary

- Build multipart bodies in memory (`mime/multipart.Writer`).
- TTS success: assert `Content-Type` and raw bytes equality from fake upstream.
- Never decode real media files from the internet.

### 12.9 What is explicitly forbidden in tests

- `http.Get("https://api.openai.com/...")` or any non-loopback host in `*_test.go` without build tag `live` (live tag **not** run in CI).
- Sleep-based synchronization.
- Relying on GOMAXPROCS or timing races.
- Reading env vars like `OPENAI_API_KEY` for unit/integration tests.
- Network in package `init()`.

### 12.10 Optional `live` build tag (not CI)

```go
//go:build live

func TestLiveOpenAIImage(t *testing.T) { ... }
```

Documented for maintainers only; **CI must not set `-tags live`**.

### 12.11 Coverage expectations by layer

| Layer | Must cover |
|-------|------------|
| config capabilities | parse, defaults, validation errors |
| router | alias, prefix, bare, capability deny |
| ingress * | golden parse/serialize per modality |
| egress * | build/parse; drop-list tests |
| proxy HTTP | matrix + usage fields (`media`, `estimated`) |
| proxy realtime | passthrough + bridge + limits |
| hooks | new fields JSON shape |

### 12.12 CI contract

```text
go vet ./...
go test ./... -race -count=1 -coverprofile=coverage.out
# coverage ≥ 90%
# binary smoke: /healthz only (no provider)
# docker build (no network pulls of secrets)
```

Optional job: run tests under `GODEBUG`/`unshare` network sandbox if available on runner — best effort.

### 12.13 Definition of Done (every multimodal PR)

- [ ] Tests written first (or same PR with clear red→green history preferred in commits)
- [ ] Matrix cells for the PR’s modality including **unsupported** paths
- [ ] Fixtures committed if new wire shapes
- [ ] `go test ./... -race` green offline
- [ ] Coverage gate green
- [ ] README/API table updated
- [ ] No new non-loopback network in tests

---

## 13. Error handling & limits

| Limit | Value |
|-------|-------|
| Body | 32 MiB (existing) |
| Upstream response header wait | 60s (existing) |
| Realtime max sessions | config `realtime.max_sessions` |
| Realtime max duration | config `realtime.max_session_minutes` |
| Shutdown | 30s drain; WS sessions closed with error frame |

Client disconnect cancels upstream context (HTTP and WS).

---

## 14. Security & privacy

- No persistence of audio/images/video beyond request lifetime.
- `key_hash` only (12 hex of sha256), never raw keys in hooks.
- Multipart peek for model remains best-effort; do not log bodies.
- Realtime bridge must not write session audio to disk.

---

## 15. Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| OpenAI-only surface for all media | Rejected by product choice B |
| Invent Anthropic Realtime WS | No real protocol; maintenance trap; hybrid forbids it |
| Separate media binary | Breaks one-binary / one-hook product |
| Always hit real APIs in CI | Not air-gapped; flaky; secret-dependent |
| Default openai_compat all capabilities on | Silent failures on vLLM/etc. |

---

## 16. Key decisions

1. **Capability-centric multimodal core** with passthrough-first translation.
2. **B + hybrid:** full HTTP multi-dialect media; realtime only real protocols + bridge.
3. **Non-colliding Anthropic vs OpenAI image paths** + header rules.
4. **openai_compat media opt-in** via `capabilities`.
5. **Usage:** tokens when known + structured `media` units; no prices in gateway.
6. **TDD + air-gapped hermetic tests** are part of the architecture, not an afterthought.
7. **Single binary**; realtime as package, not a second service.
8. **Fixtures + fakes** are the source of truth for wire contracts in CI.

---

## 17. Open questions (defaults if unanswered)

| # | Question | Default if no answer |
|---|----------|----------------------|
| Q1 | Extra provider kinds (ElevenLabs, etc.)? | Model as `openai_compat` + capabilities first; new kind only if wire ≠ OpenAI family |
| Q2 | Production WS library? | Prefer stdlib hijack + minimal framing; allow one dep if needed at PR5 |
| Q3 | Video poll billing? | Create emits media units; poll emits operational event with zero media units |
| Q4 | Mid-session realtime metering? | Session-end only for v1 |

---

## 18. PR plan (implementation order — design is complete)

Each PR is **TDD**: tests first, air-gapped, matrix cells, coverage.

| PR | Title | Scope | Depends |
|----|-------|-------|---------|
| **PR0** | Multimodal core skeleton | `Modality`, `capabilities` config + validation tests; extend `UsageEvent` + JSON tests; `internal/testutil` fakes; fixture README; capability router deny tests; no new external behavior required beyond config accept | — |
| **PR1** | Image generation general | Canonical image; 3 ingress dialects; egress openai/google; matrix tests; replace passthrough-only limitation | PR0 |
| **PR2** | Video generation general | Canonical video create/poll; 3 dialects; matrix + job id mapping tests | PR0 |
| **PR3** | Voice HTTP | TTS/STT all dialects; multipart fakes; binary response tests | PR0 |
| **PR4** | Chat audio input | `BlockAudio` translate across chat dialects; unit + proxy tests | PR0 |
| **PR5** | Realtime passthrough | OpenAI Realtime WS accept + dial openai/compat; fake WS tests; session usage | PR0 |
| **PR6** | Realtime bridge | OpenAI Realtime ↔ Google Live golden sequences | PR5 |
| **PR7** | Hardening & docs | Limits, drain, examples, README API matrix, optional OpenAPI table | PR1–6 |

PRs may stack (Graphite) but must not redesign the contract — only fill adapters and tests.

---

## 19. Success criteria

- Client can perform **text, image gen, video gen, TTS, STT** using **OpenAI, Anthropic-shaped, or Google-shaped** HTTP APIs against one gateway.
- Client can open **OpenAI Realtime** or **Google Live** sessions; cross-provider live works via bridge when configured.
- `go test ./... -race` passes **without network or real keys**.
- Coverage ≥90%; capability denials never hit upstream (asserted by fakes that `t.Fatal` on unexpected calls).
- Usage hooks receive modality + media units sufficient for external billing.

---

## 20. Approval

- [ ] Product/engineering approval of this document
- [ ] Confirm open questions Q1–Q4 or accept defaults
- [ ] Then implementation starts at **PR0** under TDD — no code before approval
