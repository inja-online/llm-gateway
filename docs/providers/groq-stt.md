# Groq STT-first multi-provider routing

**Last updated:** 2026-07-21  
**Issue:** [#92](https://github.com/inja-online/llm-gateway/issues/92)  
**Kind:** `openai_compat` · base `https://api.groq.com/openai/v1`

Use **Groq for speech-to-text** while keeping **chat on another provider** (OpenAI, Anthropic, etc.). The gateway routes by `provider/model` (or alias); capabilities gate media so chat hosts do not accidentally receive STT traffic.

## Official notes (date-stamped)

- Groq OpenAI-compatible base: `https://api.groq.com/openai/v1` — [Groq docs](https://console.groq.com/docs) — checked **2026-07**.
- Whisper-class model ids change; use the current transcription model from Groq’s catalog (example below: `whisper-large-v3`).

## Split-provider YAML

```yaml
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    api_key_env: OPENAI_API_KEY

  groq:
    kind: openai_compat
    base_url: "https://api.groq.com/openai/v1"
    api_key_env: GROQ_API_KEY
    capabilities:
      text: true                 # optional: chat on Groq if you want
      audio_transcribe: true     # required for STT routes
      audio_speech: false
      image_gen: false

defaults:
  # Bare chat model ids → OpenAI (not Groq)
  openai_dialect: openai

aliases:
  whisper-fast: groq/whisper-large-v3
```

### Why `audio_transcribe: true`?

`openai_compat` defaults to **text only**. Without `capabilities.audio_transcribe: true`, `POST /v1/audio/transcriptions` (and translations) fail closed with a capability error and **no upstream call**.

## Client STT example

Multipart transcription via the gateway, model forced to Groq:

```bash
export GROQ_API_KEY=...
# gateway has providers.groq with audio_transcribe: true

curl -s http://localhost:8787/v1/audio/transcriptions \
  -H "Authorization: Bearer $GROQ_API_KEY" \
  -F "file=@./sample.wav" \
  -F "model=groq/whisper-large-v3"
```

With alias `whisper-fast` → `groq/whisper-large-v3`:

```bash
curl -s http://localhost:8787/v1/audio/transcriptions \
  -H "Authorization: Bearer $GROQ_API_KEY" \
  -F "file=@./sample.wav" \
  -F "model=whisper-fast"
```

Chat still hits the default chat provider:

```bash
curl -s http://localhost:8787/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-4o-mini",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

## Operator checklist

- [x] YAML split providers (chat vs Groq STT)
- [x] `capabilities.audio_transcribe: true` on `groq`
- [x] Client STT curl examples (+ alias)

## Related

- [`gateway.example.yaml`](../../gateway.example.yaml) — `groq` block + `whisper-fast` alias comment
- [Compatibility matrix](../compatibility-matrix.md) — Audio STT row + operator note
- [xAI provider guide](xai.md) — same `openai_compat` capability pattern for media
