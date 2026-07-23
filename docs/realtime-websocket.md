# Realtime & Live WebSocket

Production **same-protocol** WebSocket proxy for OpenAI Realtime and Google Gemini Live.

## What it is

The gateway terminates the client WebSocket upgrade, dials the upstream (plain or **TLS/`wss`**), completes HTTP Upgrade on both sides, then **copies frames bidirectionally** until either peer closes or the session limit is hit.

| Route | Protocol family | Upstream shape |
|-------|-----------------|----------------|
| `GET /v1/realtime?model=…` | OpenAI Realtime | `{base}/realtime?model=…` |
| `GET /v1beta/models/{model}:bidiGenerateContent` | Google Live | `{base}/models/{model}:bidiGenerateContent` |
| `GET /v1/responses/ws?model=…` | OpenAI Responses duplex (experimental) | `{base}/responses?…` |

**Not implemented:** translating OpenAI Realtime events ↔ Google Live events. Cross-family attempts return **`unsupported_realtime_bridge`**.

## How it works

```
Client ──WS Upgrade──► Gateway ──TCP/TLS + Upgrade──► Upstream (api.openai.com / Gemini Live)
         ◄── frames ──►         ◄── raw frame copy ──►
```

1. Client sends `Connection: Upgrade`, `Upgrade: websocket`, `Sec-WebSocket-Key`.
2. Gateway resolves `model` → provider (alias / `provider/model` / dialect default).
3. Capability check: provider must support `realtime` (on for `kind: openai` / `google` by default; **opt-in** for `openai_compat`).
4. Session slot acquired (`realtime.max_sessions`).
5. Upstream dial:
   - `http` / `ws` → TCP port 80
   - `https` / `wss` → TLS (system roots, TLS 1.2+), port 443
6. Auth applied (`api_key_env`, `oauth2`, SA, `client_bearer`, …) on the upgrade request.
7. Bidirectional `io.Copy` of WebSocket frames (application ping/pong pass through).
8. On exit: one usage event (`modality=realtime`, `transport=websocket`, session minutes).

TCP keepalive (30s) is enabled on the upstream socket so idle load balancers are less likely to drop long sessions.

## Configuration

```yaml
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"   # TLS dial for production
    api_key_env: OPENAI_API_KEY
    # capabilities.realtime defaults true for kind: openai

  google:
    kind: google
    base_url: "https://generativelanguage.googleapis.com/v1beta"
    api_key_env: GEMINI_API_KEY

  # openai_compat must opt in:
  # grok_live:
  #   kind: openai_compat
  #   base_url: "https://api.x.ai/v1"
  #   api_key_env: XAI_API_KEY
  #   capabilities: { text: true, realtime: true }

realtime:
  max_sessions: 1024          # concurrent WS sessions per process
  max_session_minutes: 60     # hard cap; gateway closes when exceeded

defaults:
  openai_dialect: openai
  google_dialect: google
```

## How to use

### OpenAI Realtime (curl-style upgrade sketch)

```bash
# Prefer an SDK or wscat; curl alone is awkward for WS.
# Point your OpenAI Realtime client base at the gateway:

export OPENAI_BASE_URL=http://localhost:8787/v1
# model can be bare (uses defaults.openai_dialect) or provider/model
# e.g. openai/gpt-4o-realtime-preview
```

With edge auth:

```bash
# Client Authorization may be the edge key when api_key_env holds the upstream key.
# Or use auth: client_bearer and send the upstream OAuth access token as Bearer.
```

### Google Live

```
GET /v1beta/models/gemini-2.0-flash-live:bidiGenerateContent
Upgrade: websocket
```

Resolve provider via `defaults.google_dialect` or model form `google/gemini-…`.

### Session limits

| Limit | Default | Exceeded |
|-------|---------|----------|
| `max_sessions` | 1024 | HTTP **429** before upgrade |
| `max_session_minutes` | 60 | Gateway closes both sockets |

## Auth modes that work on WS

Same as HTTP: `api_key` / `api_key_env`, `bearer`, `client_bearer`, `oauth2`, `adc` / `service_account` / `token_file`. TokenSource credentials are resolved **before** the upstream upgrade request is written.

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| **501** TLS (old builds) | Upgrade to master with #105; `https` base_url is supported |
| **400** missing model | Pass `?model=` on `/v1/realtime` |
| **501** capability | Set `capabilities.realtime: true` on `openai_compat` |
| **501** `unsupported_realtime_bridge` | Do not route OpenAI Realtime model to `kind: google` (or reverse) |
| Upstream 401 | Check `api_key_env` / OAuth / SA; 401 retry applies to HTTP, not mid-WS |

## Related

- [OAuth & token sources](oauth-token-sources.md)
- [WIF recipes](wif-recipes.md)
- [SSE protocol catalog](sse-protocol-catalog.md) (HTTP streams; WS is frame passthrough)
