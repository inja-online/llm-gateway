# Anthropic `cache_control` policy (#41)

**Last updated:** 2026-07-21  
**Decision:** **Option B — passthrough only** (not modeled on canonical IR)

## Options

| Option | Meaning |
|---|---|
| **A — Implement on translate** | Carry breakpoints through IR ↔ all dialects |
| **B — PT-only (chosen)** | Preserve `cache_control` only on Anthropic→Anthropic **passthrough**; strip on translate rebuild |

## Rationale

- Prompt caching breakpoints are Anthropic-specific wire shapes (system / tools / content blocks).
- Canonical chat IR does not yet model cache breakpoints; full cross-dialect mapping is tracked under broader caching work ([#108](https://github.com/inja-online/llm-gateway/issues/108)).
- Claude Code and native Anthropic clients use **passthrough** (same-family) where fidelity is required.

## Behavior

| Path | `cache_control` |
|---|---|
| Anthropic → Anthropic passthrough | **Preserved** (JSON map; model rewrite only) |
| Anthropic → OpenAI / Google translate | **Dropped** (rebuild from IR) |
| OpenAI → Anthropic translate | **Not invented** |
| OpenAI → OpenAI passthrough | N/A (OpenAI uses different cache knobs) |

## Acceptance

- [x] Written policy Option A or B → **B**
- [x] Tests lock policy (translate drops + passthrough keeps)
- [x] Passthrough caching unbroken

## Tests

- `proxy.TestCacheControlPassthroughAnthropic`
- `proxy` translate fixtures (`anthropic_to_*` drop `cache_control`)
- Drop list: `testdata/fixtures/chat_translate/drops/common_drops.txt`
