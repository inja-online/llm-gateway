---
title: Getting started
description: Install, run, and hit /healthz
group: start
order: 1
---

## Requirements

- Go **1.25+** (to build from source), or a [release binary](https://github.com/inja-online/llm-gateway/releases) / Docker
- A YAML config (start from [`gateway.example.yaml`](https://github.com/inja-online/llm-gateway/blob/master/gateway.example.yaml))
- Provider API keys in the environment

## Build and run

```bash
git clone https://github.com/inja-online/llm-gateway.git
cd llm-gateway
go build -o llm-gateway ./cmd/gateway

cp gateway.example.yaml gateway.yaml
# edit providers / keys / hooks
export OPENAI_API_KEY=sk-...
./llm-gateway -config gateway.yaml
```

Overrides:

```bash
GATEWAY_CONFIG=/path/to/gateway.yaml
GATEWAY_LISTEN=0.0.0.0:8787
```

## Docker

```bash
docker compose up --build
# or
docker build -t llm-gateway:local .
docker run --rm -p 8787:8787 \
  -e OPENAI_API_KEY \
  -v "$PWD/gateway.yaml:/config/gateway.yaml:ro" \
  llm-gateway:local
```

## Health check

```bash
curl -s http://localhost:8787/healthz
# {"status":"ok"}
```

`/healthz` is **process liveness** only (no upstream probes). It stays open even when `edge_auth` is enabled.

## First chat (OpenAI dialect)

```bash
curl -s http://localhost:8787/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

Model routing: `aliases` → `provider/model` → dialect default. Details in the [README](https://github.com/inja-online/llm-gateway/blob/master/README.md#model-routing).

## Next

- [Claude Code setup](/llm-gateway/claude-code-checklist/)
- [Compatibility matrix](/llm-gateway/compatibility-matrix/)
- [Full HTTP API](https://github.com/inja-online/llm-gateway/blob/master/README.md#http-api)