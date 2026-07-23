# Embeddings

`POST /v1/embeddings` (OpenAI dialect) with passthrough or Google translate.

## What it is

| Upstream kind | Behavior |
|---------------|----------|
| `openai` / `openai_compat` | Passthrough to `{base}/embeddings` (model rewrite; **all body fields kept**) |
| `google` | Translate to `:embedContent` or `:batchEmbedContents` |
| `anthropic` | Not supported (400/501) |

## How it works

**Passthrough:** JSON map rewrite of `model` only → forward.  
**Google translate:**

1. Parse `input` as string or string array (token-id arrays rejected).  
2. Single input → `:embedContent`; multiple → `:batchEmbedContents`.  
3. Map response vectors back to OpenAI `data[].embedding` list shape.  
4. Record prompt tokens when Gemini usage is present.

## Fields

| Field | Passthrough | → Google |
|-------|-------------|---------|
| `model` | rewritten | path model |
| `input` | yes | text parts |
| `dimensions` | yes | `outputDimensionality` |
| `encoding_format` | yes | **`float` only** (other values → 400 on translate) |
| `task_type` | yes (opaque) | Gemini `taskType` (e.g. `RETRIEVAL_QUERY`) |
| other keys | preserved | dropped on translate |

## Examples

```bash
# OpenAI / DeepSeek / etc.
curl -sS "$GW/v1/embeddings" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/text-embedding-3-small",
    "input": "hello world",
    "dimensions": 512,
    "encoding_format": "float"
  }'
```

```bash
# Google via translate (defaults.openai_dialect can still resolve google/… models)
curl -sS "$GW/v1/embeddings" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "google/text-embedding-004",
    "input": ["doc one", "doc two"],
    "dimensions": 256,
    "task_type": "RETRIEVAL_DOCUMENT"
  }'
```

## Async batch (Google platform)

For long-running batch jobs use platform routes (not `/v1/embeddings`):

- `POST /v1beta/models/{model}:asyncBatchEmbedContent`
- `POST /v1beta/batches` job APIs  

See [platform-apis.md](platform-apis.md).

## Related

- [Chat field parity](chat-field-parity.md)
- [M6 surface notes](m6-remaining-surface.md)
