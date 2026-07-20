# Qwen (Alibaba DashScope) — regional bases

**Last updated:** 2026-07-21  
**Issue:** [#88](https://github.com/inja-online/llm-gateway/issues/88)  
**Kind:** `openai_compat` (Bearer)

DashScope’s **OpenAI-compatible mode** lives under region-specific hosts. The path includes **`compatible-mode`** — omitting it or mixing CN vs international bases with the wrong key causes auth or 404 failures.

## Official docs (date-stamped)

| Region | Base URL (example) | Vendor docs |
|---|---|---|
| **China (CN)** | `https://dashscope.aliyuncs.com/compatible-mode/v1` | [DashScope OpenAI compatible](https://help.aliyun.com/zh/model-studio/developer-reference/compatibility-of-openai-with-dashscope) — checked **2026-07** |
| **International** | `https://dashscope-intl.aliyuncs.com/compatible-mode/v1` | Same product family; use the intl console/key that matches this host — checked **2026-07** |

Always re-check Alibaba Cloud docs if hosts move; keep **`/compatible-mode/v1`** (or the current documented compatible path) in `base_url`.

## Example YAML (regional)

From [`gateway.example.yaml`](../../gateway.example.yaml):

```yaml
providers:
  # CN (default example)
  qwen:
    kind: openai_compat
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
    api_key_env: DASHSCOPE_API_KEY

  # International (uncomment if your key is intl)
  # qwen_intl:
  #   kind: openai_compat
  #   base_url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
  #   api_key_env: DASHSCOPE_API_KEY

aliases:
  # Prefer provider/model so bare ids are unambiguous in multi-provider configs
  qwen-turbo: qwen/qwen-turbo
  qwen-plus: qwen/qwen-plus
```

### Path quirks

| Topic | Guidance |
|---|---|
| `compatible-mode` | Required for OpenAI SDK-shaped Chat Completions against DashScope |
| Model ids | Often bare (`qwen-turbo`, `qwen-plus`) on upstream; through the gateway prefer `qwen/<id>` or an **alias** |
| Media | `openai_compat` media defaults **off** — opt in only if DashScope exposes that route for your account |
| Auth | Bearer via client key or `api_key_env` |

## Curl (via gateway)

```bash
export DASHSCOPE_API_KEY=...

curl -s http://localhost:8787/v1/chat/completions \
  -H "Authorization: Bearer $DASHSCOPE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen/qwen-turbo",
    "messages": [{"role": "user", "content": "ping"}]
  }'
```

Or with alias `qwen-turbo` → `qwen/qwen-turbo` as in the example above.

## Checklist

- [x] Regional examples (CN + intl)
- [x] Alias samples (`qwen-turbo`, `qwen-plus`)
- [x] README / docs index pointer

## Related

- [`gateway.example.yaml`](../../gateway.example.yaml) — `qwen` / `qwen_intl`
- [Z.AI regions](zai.md) — same pattern for another dual-region host
- [Compatibility matrix](../compatibility-matrix.md)
