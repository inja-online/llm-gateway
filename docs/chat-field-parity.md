# Chat translation field parity (#115, #162, #163)

## OpenAI request fields

| Field | Passthrough | Translate → Anthropic | Translate → Google | Notes |
|-------|-------------|----------------------|--------------------|-------|
| `messages` / multimodal | yes | yes | yes | |
| `tools` function | yes | yes | yes | |
| `tools` custom/server | yes | **error** | **error** | IR stores them (#107); OpenAI egress rebuilds |
| `tool_choice` | yes | yes | yes | |
| `temperature` / `top_p` | yes | yes | yes | |
| `max_tokens` / `max_completion_tokens` | yes | max_tokens | maxOutputTokens | field source recorded |
| `stream` / `stream_options` | yes | stream | alt=sse | stream_options OpenAI-only rebuild |
| `logprobs` / `top_logprobs` | yes | drop | drop | OpenAI egress rebuild (#115) |
| `modalities` | yes | drop | drop | |
| `prediction` | yes | drop | drop | #163 |
| `safety_identifier` | yes | drop | drop | #163 |
| `verbosity` | yes | drop | drop | #163 |
| `prompt_cache_key` / `retention` / `options` | yes | drop | drop | #108/#163 |
| `service_tier` | yes | drop | drop | |
| `user` | yes | drop | drop | |
| `reasoning_effort` | yes | thinking | thinking_config | best-effort |

## Multi-turn thinking (#106)

| Dialect | Continuity field | Behavior |
|---------|------------------|----------|
| Anthropic | `thinking` / `redacted_thinking` + signature | Passthrough + translate preserve signature |
| OpenAI-compat | `reasoning_content` | Round-trip on PT; IR `BlockThinking` on translate |
| Google | `thought` + `thoughtSignature` | Preserved on Google IR build/parse |

## Anthropic extras (#162)

`output_config`, `service_tier`, `inference_geo`, mid-conversation `system` — passthrough preserves wire JSON. Translate rebuilds only IR-mapped fields; foreign keys drop (see deprecation policy).

## Citations / grounding (#160)

`BlockCitation` holds URL/title/offsets; Google `groundingMetadata` may be stashed as raw JSON on the block. Cross-family citation invent is not performed.
