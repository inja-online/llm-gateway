# Error envelope + finish / stop reason catalog (#156)

Cross-dialect mapping used by the gateway when translating responses.

## Canonical stop reasons

| Canonical | Meaning |
|-----------|---------|
| `end_turn` | Normal completion |
| `max_tokens` | Hit token/length limit |
| `tool_use` | Model requested tool call(s) |
| `stop_sequence` | Hit a stop sequence |
| `refusal` | Safety / content filter / refusal |

## Google `finishReason` → canonical

| Google | Canonical |
|--------|-----------|
| `STOP`, `STOP_SEQUENCE` | `end_turn` |
| `MAX_TOKENS`, `LENGTH` | `max_tokens` |
| `SAFETY`, `RECITATION`, `BLOCKLIST`, `PROHIBITED_CONTENT`, `SPII`, `CONTENT_FILTER` | `refusal` |
| `MALFORMED_FUNCTION_CALL` | `tool_use` |
| `OTHER`, empty, unspecified | `end_turn` |
| Any candidate with `functionCall` parts | `tool_use` (overrides STOP) |

## OpenAI `finish_reason` → canonical

| OpenAI | Canonical |
|--------|-----------|
| `stop` | `end_turn` |
| `length` | `max_tokens` |
| `tool_calls` / `function_call` | `tool_use` |
| `content_filter` | `refusal` |
| `null` / empty (stream mid-chunk) | unset until final |

## Anthropic `stop_reason` → canonical

| Anthropic | Canonical |
|-----------|-----------|
| `end_turn` | `end_turn` |
| `max_tokens` | `max_tokens` |
| `tool_use` | `tool_use` |
| `stop_sequence` | `stop_sequence` |
| `refusal` | `refusal` |

## Error envelopes

| Dialect | Shape |
|---------|-------|
| OpenAI | `{"error":{"message":"...","type":"...","code":"..."}}` |
| Anthropic | `{"type":"error","error":{"type":"...","message":"..."}}` |
| Google | `{"error":{"code":N,"message":"...","status":"..."}}` |

Gateway maps upstream errors when translating; passthrough preserves the upstream body.
