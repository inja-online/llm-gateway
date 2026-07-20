# Non-function OpenAI tools policy (#49)

**Last updated:** 2026-07-21  
**Decision:** **error** on the **translation** path (not silent skip)

## Policy

| Path | Non-function `tools[].type` (e.g. `custom`, `file_search`, server tools) |
|---|---|
| **Passthrough** (OpenAI → `openai` / `openai_compat`) | **Forward as-is** (JSON map; model rewrite only). Upstream may accept or reject. |
| **Translation** (OpenAI → Anthropic/Google via canonical IR) | **HTTP 400** `invalid_request_error` — only `type: "function"` (or empty type, treated as function) is supported. |

**Rationale:** silent skip made agents believe tools were registered when they were not. Fail closed on translate so clients fix the tool set or use passthrough to a host that supports custom tools.

## Function tools

Unchanged: name required, parameters JSON schema carried on IR, rebuilt per dialect.

## Acceptance

- [x] Document skip \| error \| warn → **error** (translate) / forward (passthrough)
- [x] Unit tests lock policy (`ingress/openai`, `proxy`)
- [x] Function tools unchanged

## Related

- Future expansion: [#107](https://github.com/inja-online/llm-gateway/issues/107) tool union beyond function-only
- Custom grammar tools: [#161](https://github.com/inja-online/llm-gateway/issues/161)
