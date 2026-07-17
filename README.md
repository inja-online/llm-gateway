# llm-gateway

A small, dependency-free LLM gateway. Clients speak **OpenAI** or **Anthropic** API dialects; the gateway routes to any upstream provider (OpenAI, Anthropic, Google, DeepSeek, xAI, OpenRouter, any OpenAI-compatible host) and reports token usage through **hooks** — no database, no auth layer, one binary.

```
your app (OpenAI SDK / Anthropic SDK / Claude Code)
        │
        ▼
   llm-gateway  ──► usage events (JSONL / webhook / Go hook)
        │
        ▼
  any upstream provider
```

## Design principles

- **No auth.** The gateway validates nothing. Your client's API key is forwarded to the upstream provider as-is (mapped to the provider's auth scheme). Optionally, a provider can be configured with `api_key_env` so the gateway supplies the key server-side.
- **No database.** Metering is push-only: every request emits exactly one usage event to the configured hooks. Pipe the JSONL anywhere; POST it anywhere; or embed the gateway as a Go library and register your own hook.
- **Modular.** Dialects (ingress wire formats) and providers (egress) are self-contained packages. Adding one doesn't touch the core.
- **Passthrough first.** When the client dialect matches the upstream dialect, bytes are forwarded near-verbatim — full fidelity, minimal surface for translation bugs.

## Status

Early. Currently implemented (M1):

- `POST /v1/chat/completions` → any OpenAI-compatible upstream (passthrough), streaming + non-streaming
- Model routing: `provider/model` prefixes, aliases, per-dialect defaults
- Usage extraction from upstream `usage` fields (incl. `stream_options.include_usage` auto-injection)
- JSONL hook, in-process Go hook
- `GET /healthz`

Roadmap: Anthropic dialect (`/v1/messages`) + Claude Code support, cross-dialect translation, tool-call mapping, Google egress, webhook hook. See plan in commit history.

## Quickstart

```bash
go build -o llm-gateway ./cmd/gateway
./llm-gateway -config gateway.yaml
```

`gateway.yaml`:

```yaml
listen: ":8787"
providers:
  deepseek:   { kind: openai_compat, base_url: "https://api.deepseek.com" }
  openrouter: { kind: openai_compat, base_url: "https://openrouter.ai/api/v1" }
  openai:     { kind: openai,        base_url: "https://api.openai.com/v1" }
defaults:
  openai_dialect: openai   # bare model ids on /v1/chat/completions go here
aliases:
  fast: deepseek/deepseek-chat
hooks:
  jsonl: { output: stdout }
```

Point any OpenAI SDK at it:

```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:8787/v1", api_key="<your DEEPSEEK key>")
r = client.chat.completions.create(model="deepseek/deepseek-chat",
                                   messages=[{"role": "user", "content": "hi"}])
```

Every request prints one usage line:

```json
{"request_id":"req_1a2b3c","time":"2026-07-18T12:00:00Z","dialect_in":"openai","provider":"deepseek","model":"deepseek/deepseek-chat","upstream_model":"deepseek-chat","tokens_in":12,"tokens_out":5,"estimated":false,"stream":false,"status":"ok","http_status":200,"latency_ms":812,"key_hash":"9f8e7d6c5b4a"}
```

## Key forwarding — read this

The API key you send to the gateway is forwarded to whichever provider the model routes to. **The key must be valid for the target provider.** If you send an OpenAI key and route to `anthropic/...`, the upstream will reject it. For server-held keys, set `api_key_env` on the provider and export the variable.

`base_url` convention: include the version prefix (`https://api.openai.com/v1`); the gateway appends `/chat/completions`.

## Hooks

| Hook | Config | Behavior |
|---|---|---|
| JSONL | `hooks.jsonl.output: stdout \| stderr \| /path/file` | One JSON line per request |
| Webhook | `hooks.webhook.url` | Async POST per event (roadmap) |
| Go interface | `gateway.New(cfg, gateway.WithHook(h))` | In-process, for embedding |

Event schema: see `hooks/hooks.go` (`UsageEvent`). Invariant: exactly one event per request, including error and abort paths. `estimated: true` means the upstream reported no usage — tokens are zero, not measured.

## Library use

```go
import (
    gateway "github.com/mamad/llm-gateway"
    "github.com/mamad/llm-gateway/config"
    "github.com/mamad/llm-gateway/hooks"
)

cfg, _ := config.Load("gateway.yaml")
h, _ := gateway.New(cfg, gateway.WithHook(hooks.Func(func(ctx context.Context, ev hooks.UsageEvent) {
    // your billing / analytics
})))
http.ListenAndServe(cfg.Listen, h)
```

## License

MIT
