# llm-gateway

[![ci](https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/inja-online/llm-gateway/actions/workflows/ci.yml)

A small, dependency-free LLM gateway. Clients speak **OpenAI** or **Anthropic** API dialects; the gateway routes to any upstream provider (OpenAI, Anthropic, DeepSeek, xAI, OpenRouter, any OpenAI-compatible host) and reports token usage through **hooks** — no database, no auth layer, one binary.

```
your app (OpenAI SDK / Anthropic SDK / Claude Code)
        │
        ▼
   llm-gateway  ──► usage events (JSONL stdout / webhook / Go hook)
        │
        ▼
  any upstream provider
```

**Stateless and cloud-native:** no database, no sessions, no sticky routing. Scale with identical replicas; emit usage to stdout or a webhook. One static binary (or container) runs the same on a laptop, a Windows box, Docker, or Kubernetes.

## Design principles

- **No auth.** The gateway validates nothing. Your client's API key is forwarded to the upstream provider as-is (mapped to the provider's auth scheme). Optionally, a provider can be configured with `api_key_env` so the gateway supplies the key server-side.
- **No database.** Metering is push-only: every request emits exactly one usage event to the configured hooks. Pipe the JSONL anywhere, POST a webhook, or embed the gateway as a Go library and register your own hook.
- **Stateless.** Any replica can handle any request. Config is load-once; graceful SIGTERM drain is built in.
- **Modular.** Dialects (ingress wire formats) and providers (egress) are self-contained packages. Adding one doesn't touch the core.
- **Passthrough first.** When the client dialect matches the upstream dialect, bytes are forwarded near-verbatim — full fidelity, minimal surface for translation bugs.
- **Simple deploy.** One binary, one YAML file, optional Docker/K8s manifests — no control plane.

## Status

Working, early. Two client dialects in, two provider families out — any combination.

| Client speaks | Upstream is | Path |
|---|---|---|
| OpenAI (`POST /v1/chat/completions`) | `openai` / `openai_compat` | passthrough |
| OpenAI | `anthropic` | translated |
| Anthropic (`POST /v1/messages`) | `anthropic` | passthrough |
| Anthropic | `openai` / `openai_compat` | translated |

Covered on every chat path: streaming and non-streaming, tool calls, images, system prompts, error envelopes reshaped into the caller's dialect, and one usage event per request.

Also implemented: `POST /v1/messages/count_tokens`, `GET /healthz`.

**Roadmap:** Google/Gemini egress, `GET /v1/models`. Config already accepts `kind: google`, but chat translation for it is not implemented yet.

---

## Quickstart

### Binary (macOS / Linux / Windows)

```bash
git clone https://github.com/inja-online/llm-gateway.git
cd llm-gateway
go build -o llm-gateway ./cmd/gateway          # Windows: go build -o llm-gateway.exe ./cmd/gateway
cp gateway.example.yaml gateway.yaml
# edit gateway.yaml
./llm-gateway -config gateway.yaml            # Windows: .\llm-gateway.exe -config gateway.yaml
```

Or download a release binary for your OS/arch from GitHub Releases.

Env overrides (optional):

| Env | Meaning |
|---|---|
| `GATEWAY_CONFIG` | Default path for `-config` |
| `GATEWAY_LISTEN` | Bind address (overrides `listen:` in YAML) |

Default listen address is `:8787` if neither YAML nor env sets it. SIGINT / SIGTERM shut down gracefully (30s drain for in-flight requests).

### Docker

```bash
docker compose up --build
curl http://localhost:8787/healthz
```

Mount your own config: edit the volume in `docker-compose.yml`, or:

```bash
docker build -t llm-gateway .
docker run --rm -p 8787:8787 \
  -v "$PWD/gateway.yaml:/config/gateway.yaml:ro" \
  -e GATEWAY_LISTEN=:8787 \
  llm-gateway
```

### Kubernetes

```bash
# Point the Deployment image at your registry build, then:
kubectl apply -f deploy/k8s/gateway.yaml
kubectl -n llm-gateway port-forward svc/llm-gateway 8787:8787
```

Replicas share nothing. Prefer `hooks.jsonl.output: stdout` (default in the manifest) and your cluster log shipper, or set `hooks.webhook.url` for multi-replica billing.

### Windows PowerShell client smoke

```powershell
$env:KEY = "sk-..."
.\examples\curl-openai.ps1
```

Example config:

```yaml
listen: ":8787"
providers:
  deepseek:   { kind: openai_compat, base_url: "https://api.deepseek.com" }
  openrouter: { kind: openai_compat, base_url: "https://openrouter.ai/api/v1" }
  openai:     { kind: openai,        base_url: "https://api.openai.com/v1" }
  anthropic:  { kind: anthropic,     base_url: "https://api.anthropic.com/v1" }
defaults:
  openai_dialect: openai       # bare model ids on /v1/chat/completions
  anthropic_dialect: anthropic # bare model ids on /v1/messages
aliases:
  fast: deepseek/deepseek-chat
hooks:
  jsonl: { output: stdout }
```

Point any OpenAI SDK at it:

```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key="<key valid for target provider>")
r = client.chat.completions.create(
    model="deepseek/deepseek-chat",
    messages=[{"role": "user", "content": "hi"}],
)
```

…or any Anthropic SDK, including against a non-Anthropic model:

```python
from anthropic import Anthropic
client = Anthropic(base_url="http://localhost:8787", api_key="<key for the target provider>")
r = client.messages.create(
    model="deepseek/deepseek-chat",
    max_tokens=100,
    messages=[{"role": "user", "content": "hi"}],
)
```

Shell examples live under [`examples/`](examples/):

```bash
KEY=sk-... MODEL=deepseek/deepseek-chat ./examples/curl-openai.sh
KEY=sk-ant-... ./examples/claude-code.sh
```

Every successful (and failed) chat request prints one usage line when JSONL is configured:

```json
{"request_id":"req_1a2b3c","time":"2026-07-18T12:00:00Z","dialect_in":"openai","provider":"deepseek","model":"deepseek/deepseek-chat","upstream_model":"deepseek-chat","tokens_in":12,"tokens_out":5,"estimated":false,"stream":false,"status":"ok","http_status":200,"latency_ms":812,"key_hash":"9f8e7d6c5b4a"}
```

---

## HTTP API

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/chat/completions` | OpenAI Chat Completions dialect |
| `POST` | `/v1/messages` | Anthropic Messages dialect |
| `POST` | `/v1/messages/count_tokens` | Token count (proxy or estimate) |
| `GET` | `/healthz` | Liveness: `{"status":"ok"}` |

There is no `/v1/models` yet. Unknown routes return the Go `net/http` default 404.

### Request size and timeouts

- Request and response bodies are capped at **32 MiB**.
- The HTTP client has **no overall request timeout** (streams can be long-lived). Dial and response-header timeouts use the transport defaults (`ResponseHeaderTimeout` is 60s). Client disconnect cancels the upstream context.
- `count_tokens` proxying to Anthropic uses a **15s** timeout.

---

## Model routing

The public `model` field is resolved in this order:

1. **Alias table** — exact match in `aliases` (e.g. `fast` → `deepseek/deepseek-chat`).
2. **`provider/model` prefix** — first path segment must name a configured provider; the rest is the upstream model id (`openai/gpt-4o` → provider `openai`, upstream `gpt-4o`).
3. **Bare id** — no slash; uses the dialect default:
   - OpenAI requests → `defaults.openai_dialect`
   - Anthropic requests → `defaults.anthropic_dialect`

If a bare id has no default, the gateway returns **404** with a dialect-shaped error. Unknown providers also return **404**.

Examples with the sample config:

| Client `model` | Dialect | Provider | Upstream model |
|---|---|---|---|
| `deepseek/deepseek-chat` | either | `deepseek` | `deepseek-chat` |
| `fast` | either | `deepseek` | `deepseek-chat` |
| `gpt-4o` | OpenAI | `openai` (default) | `gpt-4o` |
| `claude-sonnet-4-20250514` | Anthropic | `anthropic` (default) | `claude-sonnet-4-20250514` |

---

## Configuration reference

Single YAML file. Unknown fields are rejected (`KnownFields(true)`).

| Field | Required | Description |
|---|---|---|
| `listen` | no | Bind address; default `:8787` |
| `providers` | yes | Map of name → provider (at least one) |
| `providers.<name>.kind` | yes | `openai`, `openai_compat`, `anthropic`, or `google` |
| `providers.<name>.base_url` | yes | Origin **including version prefix**; trailing slash is trimmed |
| `providers.<name>.api_key_env` | no | Env var name; when set and non-empty, replaces the client key |
| `defaults.openai_dialect` | no | Provider name for bare models on OpenAI ingress |
| `defaults.anthropic_dialect` | no | Provider name for bare models on Anthropic ingress |
| `aliases` | no | Map of public id → `provider/upstream-model` |
| `hooks.jsonl.output` | no | `stdout`, `stderr`, or a file path (append mode) |
| `hooks.webhook.url` | no | Async POST of each usage event as JSON |
| `hooks.webhook.timeout` | no | Per-post timeout; default `3s` |

### Provider kinds

| Kind | Upstream path | Auth header | Notes |
|---|---|---|---|
| `openai` | `{base_url}/chat/completions` | `Authorization: Bearer …` | Official OpenAI |
| `openai_compat` | same | same | DeepSeek, OpenRouter, xAI, vLLM, etc. |
| `anthropic` | `{base_url}/messages` | `x-api-key` + `anthropic-version: 2023-06-01` | Official Anthropic |
| `google` | — | `x-goog-api-key` | Config-only today; chat translation not implemented |

`base_url` must include the API version segment. Examples:

- `https://api.openai.com/v1` → gateway posts to `…/v1/chat/completions`
- `https://api.anthropic.com/v1` → gateway posts to `…/v1/messages`
- `https://api.deepseek.com` → posts to `…/chat/completions` (DeepSeek's layout)

### Auth and key forwarding

The gateway does **not** authenticate callers. It extracts a credential from:

1. `Authorization: Bearer <key>`, or
2. `x-api-key: <key>`

That value is forwarded to the resolved provider, remapped to the provider's scheme (table above). **The key must be valid for the target provider.** Sending an OpenAI key while routing to `anthropic/...` will fail at the upstream.

If `api_key_env` is set on the provider and the env var is non-empty, that value **replaces** the client key entirely (useful for server-held keys while clients send a dummy or internal token).

`key_hash` on usage events is the first 12 hex chars of SHA-256 of the credential that was selected for forwarding — enough to correlate usage per key without storing the secret.

---

## Passthrough vs translation

### Passthrough (same family)

When the client dialect matches the provider kind:

1. Body is parsed as generic JSON (not fully re-validated).
2. Only `model` is rewritten to the upstream model id.
3. For OpenAI streaming, `stream_options.include_usage` is set to `true` if the client did not already set it (so usage can be metered).
4. Bytes are forwarded; SSE is relayed line-by-line while scanning for usage.
5. Upstream HTTP ≥400 responses are relayed with status and body largely unchanged.

This is the full-fidelity path (Claude Code → Anthropic provider).

### Translation (cross family)

When dialects differ, the request is parsed into a **canonical** form (Anthropic-shaped content blocks — the structural superset), then built into the upstream wire format. The response (or stream) is converted back into the caller's dialect.

| Feature | Supported in translation |
|---|---|
| Text messages | yes |
| System / developer prompts | yes (`developer` → system) |
| Multimodal images (URL or base64) | yes |
| Tool definitions + tool choice | yes (`required` ↔ Anthropic `any`) |
| Tool calls / tool results | yes |
| Streaming | yes |
| Temperature, top_p, stop sequences | yes |
| `max_tokens` / `max_completion_tokens` | yes (OpenAI either field) |
| Thinking / reasoning blocks | carried when present on Anthropic wire |
| OpenAI `n`, `logprobs`, `response_format`, `seed`, etc. | **not** mapped — dropped on translate |
| Non-function OpenAI tools | skipped |

Canonical is only used on the translation path; same-dialect traffic never touches it.

**Streaming token-display caveat (Anthropic client → OpenAI-compatible upstream):** Anthropic clients expect `input_tokens` on `message_start`. OpenAI-wire upstreams typically only report usage at the end of the stream. On that path `message_start` carries `input_tokens: 0`; true counts appear in the final event and in the usage hook. Claude Code works; only its live token display is wrong mid-stream.

**Anthropic requires `max_tokens`:** OpenAI clients that omit it get a default of **4096** when translating to Anthropic.

**Errors:** gateway-generated errors use the caller's dialect envelope. Upstream ≥400 bodies are reshaped into the caller's envelope on the translation path; on passthrough they are forwarded as-is.

---

## `count_tokens`

`POST /v1/messages/count_tokens` exists so clients that call it before a request (Claude Code among them) do not get a hard 404.

| Resolved provider | Behavior |
|---|---|
| `anthropic` | Proxy to `{base_url}/messages/count_tokens` for an exact count; on transport/4xx failure, fall back to local estimate |
| anything else | Local estimate only |

Local estimate: roughly **one token per four characters** of system text, message text, tool schemas, and tool results. It is **not** for billing — only a usable number for clients that require the endpoint. No usage event is emitted for `count_tokens`.

---

## Hooks and usage events

### Sinks

| Hook | Config | Behavior |
|---|---|---|
| JSONL | `hooks.jsonl.output: stdout \| stderr \| /path` | One JSON line per chat request; file opened append mode |
| Webhook | `hooks.webhook.url` (+ optional `timeout`, default 3s) | Async POST of the same JSON event; failures are logged only |
| Go | `gateway.WithHook(h)` | In-process; called after the response completes |

Multiple hooks fan out via `hooks.Multi`. The proxy calls `OnUsage` after the response finishes (or aborts). **JSONL/Go hooks must not block.** The webhook sink posts in a background goroutine so a slow billing endpoint never stalls the client.

**Cloud metering:** use `stdout` (or webhook), never a shared local file across replicas.

### Invariant

Exactly **one** `UsageEvent` per proxied **chat** request (`/v1/chat/completions` and `/v1/messages`), including error and client-abort paths. `count_tokens` and `/healthz` do not emit events.

### Event schema

```json
{
  "request_id": "req_<16 hex>",
  "time": "RFC3339",
  "dialect_in": "openai | anthropic",
  "provider": "configured provider name",
  "model": "public model id as the client sent it",
  "upstream_model": "id sent upstream after routing",
  "tokens_in": 0,
  "tokens_out": 0,
  "estimated": false,
  "stream": false,
  "status": "ok | upstream_error | client_abort | bad_request",
  "http_status": 200,
  "latency_ms": 0,
  "ttft_ms": 0,
  "key_hash": "12 hex chars or empty"
}
```

| Field | Meaning |
|---|---|
| `estimated` | `true` when the upstream reported no usage; token fields are then zero (or incomplete), not measured |
| `ttft_ms` | Time to first stream byte; omitted / zero for non-stream |
| `status` | Outcome class (see table) |
| `http_status` | Status written to the client (`499` for client abort without a response) |
| `key_hash` | Short hash of the forwarded credential; empty if no key |

`status` values:

| Value | When |
|---|---|
| `ok` | Successful completion |
| `bad_request` | Client/input/routing error (missing model, unknown provider, parse failure, …) |
| `upstream_error` | Upstream transport failure or HTTP ≥400 / incomplete stream |
| `client_abort` | Client canceled the request context mid-flight |

Type definition: [`hooks/hooks.go`](hooks/hooks.go) (`UsageEvent`).

---

## Claude Code

Point it at the gateway — no code changes:

```bash
export ANTHROPIC_BASE_URL=http://localhost:8787
export ANTHROPIC_API_KEY=<key valid for whatever provider you route to>
# optional: export ANTHROPIC_MODEL=deepseek/deepseek-chat
claude
```

Or use [`examples/claude-code.sh`](examples/claude-code.sh).

With an Anthropic-kind provider this is byte-exact passthrough. Route to an OpenAI-compatible provider and requests are translated both ways (see streaming token-display caveat above).

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

YAML-configured hooks (JSONL) are wired automatically; `WithHook` adds more.

Standalone binary: [`cmd/gateway`](cmd/gateway) — `gateway -config gateway.yaml`.

---

## Architecture

```
cmd/gateway/          standalone binary
gateway.go            library entry: New(cfg, opts...) → http.Handler

proxy/                HTTP pipeline: route → forward → meter
canonical/            dialect-neutral request/response/stream types
ingress/openai/       OpenAI wire → canonical (+ serialize, stream, errors)
ingress/anthropic/    Anthropic wire → canonical
egress/openai/        canonical → OpenAI wire
egress/anthropic/     canonical → Anthropic wire
hooks/                UsageEvent + Hook; hooks/jsonl sink
config/               YAML load and validation
internal/sse/         SSE line scanner
```

Cross-dialect path: `ingress.Parse` → `canonical` → `egress.Build` → upstream → `egress.Parse` / stream parser → `ingress.Serialize`.

Same-dialect path: rewrite `model` (+ stream usage option) → forward bytes.

---

## Deploy layout

```
Dockerfile              multi-stage distroless image (non-root)
docker-compose.yml      one-command local stack
deploy/k8s/gateway.yaml Namespace + ConfigMap + Deployment + Service
cmd/gateway             binary entrypoint (graceful shutdown)
gateway.example.yaml    sample config
```

Same binary and config shape everywhere. No special cloud mode.

## Development

```bash
go test ./...
go test -race ./...
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1
go vet ./...
docker build -t llm-gateway:dev .
```

CI (`.github/workflows/ci.yml`) on every push/PR:

- `go build`, `go vet`, `go test -race`
- coverage profile with a **≥ 90%** gate
- binary smoke: start the server and hit `/healthz`
- Docker image build

Release (`.github/workflows/release.yml`) on `v*` tags: multi-arch binaries
(`linux`/`darwin`/`windows` × `amd64`/`arm64`) + checksums attached to a
GitHub Release.

Only runtime dependency: `gopkg.in/yaml.v3`.

---

## License

MIT
