# SSE protocol catalog (#157)

Gateway streaming behavior by dialect. Same-family traffic is **byte-passthrough** after model rewrite; only cross-dialect translate rebuilds events.

## OpenAI Chat Completions (`text/event-stream`)

| Event / data | Notes |
|--------------|-------|
| `data: {choices:[{delta:{…}}]}` | Token deltas; `role` on first chunk |
| `delta.reasoning_content` | OpenAI-compat thinking (DeepSeek/Kimi/…); mapped to thinking blocks on translate |
| `delta.tool_calls` | Incremental tool call args |
| `finish_reason` | On final choice chunk |
| `data: [DONE]` | Stream terminator |

## OpenAI Responses API

| Event type | Notes |
|------------|-------|
| `response.created` / `response.in_progress` / `response.completed` | Lifecycle |
| `response.output_text.delta` | Text deltas |
| `response.function_call_arguments.delta` | Tool args |
| Passthrough | Gateway does not rewrite Responses SSE on openai family |

## Anthropic Messages

| Event `type` | Notes |
|--------------|-------|
| `message_start` | Message skeleton + usage start |
| `content_block_start` / `delta` / `stop` | Text, tool_use, thinking blocks |
| `message_delta` | `stop_reason`, output tokens |
| `message_stop` | End |
| `ping` | Keepalive (forwarded on passthrough) |

## Google Gemini (`alt=sse`)

| Shape | Notes |
|-------|-------|
| JSON chunk per `data:` line | `candidates[].content.parts[]` |
| `thought: true` parts | Thinking; `thoughtSignature` preserved for multi-turn (#106) |
| `functionCall` parts | Tool use |
| Final chunk may include `usageMetadata` | Mapped to usage when present |

## Gateway policy

- Passthrough: do not strip unknown event types or fields.
- Translate: rebuild only known IR fields; unknown vendor events are not invented.
- Mid-stream client disconnect → usage `client_abort` (499 semantics).
