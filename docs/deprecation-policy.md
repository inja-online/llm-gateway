# Deprecation & field-drop policy

**Last updated:** 2026-07-18

This document defines how **Inja LLM Gateway** handles removed features and **fields dropped during cross-dialect translation**, so operators and agents are not surprised by silent data loss.

## Passthrough vs translation

| Path | Field policy |
|---|---|
| **Passthrough** (same family: OpenAI→openai/openai_compat, Anthropic→anthropic, Google→google) | **Never drop** request/response JSON fields. Only rewrite `model` (and stream usage injection where documented). Unknown OpenRouter/xAI extras must survive. |
| **Translation** (cross family via `canonical`) | **May drop** vendor-specific fields that have no canonical mapping. Drops must be covered by explicit tests (drop lists / goldens). |

Passthrough regressions that strip headers or body keys are **bugs**, not deprecations.

## Documented chat translation drops (today)

On OpenAI ↔ Anthropic ↔ Google **chat** translation, the following OpenAI-oriented knobs are **not** mapped (dropped on translate path only):

- `n`, `logprobs`, `top_logprobs`, `response_format`, `seed`, and similar vendor extras without a canonical home

Thinking / tool / multimodal blocks that *are* mapped are listed in the README “Passthrough vs translation” table. New drops require a changelog **Changed** or **Removed** entry.

## Warning headers (decision)

| Mechanism | Status |
|---|---|
| HTTP `Warning` header | **Not used** (ambiguous, often stripped by intermediaries) |
| Custom `x-gateway-dropped-fields` | **Not implemented in v1** — reserved for a future optional opt-in |
| Hooks / metrics counters for drops | **Not implemented in v1**; prefer tests + changelog |

**Rationale:** translation drop lists are stable and tested; spamming large headers with field names on every request is costly and rarely consumed by SDKs. If product needs runtime observability later, add opt-in `x-gateway-dropped-fields: field1,field2` (names only, never payloads) behind config, MINOR release.

Until then:

1. Keep **explicit drop-list tests** next to translators.
2. Document behavioral changes in `CHANGELOG.md`.
3. Prefer fail-closed errors for **unsupported tool types / modalities** over silent skip when fidelity matters (policy may evolve per modality).

## Media / realtime

- Image/video/audio **Extra** maps: unknown keys may live in `Extra` or be dropped per design-spec drop lists; goldens under `testdata/fixtures/` are authoritative when present.
- **Realtime bridge (deferred):** full OpenAI Realtime ↔ Google Live IR is **not implemented**. Cross-protocol attempts fail closed with `unsupported_realtime_bridge`. Same-protocol passthrough only.
- **Realtime bridge drop list (unmapped until bridge ships):** all cross-protocol event names, audio format conversion, tool/function remapping, VAD/session extras — never half-apply; never invent Anthropic WebSocket dialect.

## Semver rules for drop behavior

| Change | Version |
|---|---|
| Start dropping a field that was previously preserved on a **translation** path | **MAJOR** (or MINOR only if field was never documented as supported — still changelog) |
| Preserve a field that was previously dropped | **MINOR** (additive fidelity) |
| Passthrough begins stripping a field/header | **PATCH bugfix** (restore) + regression test |
| Remove a public route or required config field | **MAJOR** |

Deprecation timeline for intentional removals:

1. **Announce** in changelog under Changed (deprecated) with replacement.
2. Keep working for **at least one MINOR** (preferably one MAJOR boundary for wire breaks).
3. **Remove** in a MAJOR with Removed section.

## Examples

**OpenRouter plugins body:** passthrough — `plugins` / `provider` must reach upstream unchanged (except `model` rewrite).

**Image Extra:** if canonical image IR cannot express a vendor `quality` enum value, document in drop list and tests; do not silently change meaning of another field.

**Realtime unmapped event:** drop-list test asserts the event is not re-encoded as a wrong type; bridge may close with a clear error rather than half-apply.

## Related

- [CHANGELOG.md](changelog.md)
- [docs/compatibility-matrix.md](compatibility-matrix.md)
- [docs/sdk-compatibility-matrix.md](sdk-compatibility-matrix.md)
- [README.md](https://github.com/inja-online/llm-gateway/blob/master/README.md) HTTP API and translation tables
