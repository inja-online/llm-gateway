# Media contract golden fixtures

Air-gapped wire samples for **Gateway Media Contract v1**. CI loads these with
`go test` only — never regenerates over the network.

## Naming policy

```
{dialect}_{modality}_{case}.json
```

| Segment | Values |
|---|---|
| `dialect` | `openai`, `anthropic`, `google` |
| `modality` | `image_gen`, `video_gen`, `audio_speech`, `audio_transcribe` |
| `case` | `request`, `response`, `create_request`, `create_response`, `poll_response`, `error`, … |

Files live under this directory (optionally grouped by dialect subfolder for
readability). The basename still follows the policy above.

Examples:

- `openai_image_gen_request.json` — OpenAI `POST /v1/images/generations` body
- `anthropic_image_gen_response.json` — Anthropic-gateway `image_generation` envelope
- `google_video_gen_create_request.json` — Gemini `:generateVideos` / predict body
- `openai_audio_speech_request.json` — OpenAI TTS `POST /v1/audio/speech`
- `openai_error_unsupported_capability.json` — dialect error envelope sample

## Rules

1. **Tiny base64 only** — use `YQ==` (the letter `a`). No production media.
2. **No PII / no live captures** unless sanitized offline before commit.
3. **Offline production** — hand-authored from public schema docs or from local
   unit tests. A `fixtures` build tag must never hit the public internet.
4. **Shape stability** — fixtures lock parse/serialize round-trips; prefer key
   presence checks over raw byte equality when serializers inject timestamps.
5. **Multipart STT** — prefer building multipart bodies in-test; optional JSON
   peek samples may live here for speech/transcribe.

## Layout

```
media/
  README.md
  openai/
    openai_image_gen_request.json
    openai_image_gen_response.json
    openai_video_gen_create_request.json
    openai_video_gen_create_response.json
    openai_audio_speech_request.json
  anthropic/
    anthropic_image_gen_request.json
    anthropic_image_gen_response.json
    anthropic_video_gen_create_request.json
    anthropic_video_gen_create_response.json
  google/
    google_image_gen_request.json
    google_image_gen_response.json
    google_video_gen_create_request.json
    google_video_gen_create_response.json
  errors/
    openai_error_unsupported_capability.json
    anthropic_error_unsupported_capability.json
    google_error_unsupported_capability.json
```

## Running

```bash
go test ./proxy/ -run TestMediaFixtures -count=1
```
