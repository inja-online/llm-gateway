# Deprecation & field-drop policy

**Last updated:** 2026-07-21  
**Issue:** [#103](https://github.com/inja-online/llm-gateway/issues/103)  
**Related:** [#152](https://github.com/inja-online/llm-gateway/issues/152) (optional `x-gateway-dropped-fields` implementation)

This document defines how **Inja LLM Gateway** handles removed features and **fields dropped during cross-dialect translation**, so operators and agents are not surprised by silent data loss.

## Acceptance criteria (#103)

| Criterion | Status |
|---|---|
| Passthrough never drops request/response JSON fields (except documented model rewrite / stream usage injection) | **Policy + tests** |
| Decision on HTTP `Warning` vs `x-gateway-dropped-fields` | **Decided** (see below) |
| Metrics / hooks for drops | **Optional / not in v1** |
| Semver rules when drop behavior changes | **Documented** |

## Passthrough vs translation

| Path | Field policy |
|---|---|
| **Passthrough** (same family: OpenAI→openai/openai_compat, Anthropic→anthropic, Google→google) | **Never drop** request/response JSON fields. Only rewrite `model` (and stream usage injection where documented). Unknown OpenRouter/xAI extras must survive. |
| **Translation** (cross family via `canonical`) | **May drop** vendor-specific fields that have no canonical mapping. Drops must be covered by explicit tests (drop lists / goldens). |

Passthrough regressions that strip headers or body keys are **bugs**, not deprecations.

**Owning tests (hermetic):**

- OpenRouter extras: `proxy/openrouter_test.go` (`plugins` / `provider` preserved)
- Reasoning content passthrough: `proxy/reasoning_passthrough_test.go`
- Media same-family: `proxy/media.go` comments + media tests
- Translate drop list fixture: `testdata/fixtures/chat_translate/drops/common_drops.txt` + `proxy/translate_fixtures_test.go`
- Policy doc lock: `proxy/deprecation_policy_doc_test.go`

## Documented chat translation drops (today)

On OpenAI ↔ Anthropic ↔ Google **chat** translation, the following OpenAI-oriented knobs are **not** mapped (dropped on translate path only) — see also `common_drops.txt`:

- `n` > 1 (policy: single choice; multi-choice rejected/dropped per fixtures)
- `logprobs`, `top_logprobs`, `logit_bias`
- `stream_options` on non-OpenAI egress
- Anthropic `cache_control` breakpoints when **translating** (passthrough Anthropic→Anthropic may still strip depending on path — tracked under fidelity issues)
- Other vendor-only extras without a canonical home

Thinking / tool / multimodal blocks that *are* mapped are listed in the README “Passthrough vs translation” table. New drops require a changelog **Changed** or **Removed** entry.

## Warning headers (decision)

| Mechanism | Status |
|---|---|
| HTTP `Warning` header | **Not used** (ambiguous, often stripped by intermediaries) |
| Custom `x-gateway-dropped-fields` | **Not implemented in v1** — reserved for a future optional opt-in ([#152](https://github.com/inja-online/llm-gateway/issues/152)) |
| Hooks / metrics counters for drops | **Not implemented in v1**; prefer tests + changelog. Optional metrics exporter is separate product work. |

**Rationale:** translation drop lists are stable and tested; spamming large headers with field names on every request is costly and rarely consumed by SDKs. If product needs runtime observability later, add opt-in `x-gateway-dropped-fields: field1,field2` (names only, never payloads) behind config, **MINOR** release.

Until then:

1. Keep **explicit drop-list tests** next to translators (`common_drops.txt` + goldens).
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
- Fixture: [`testdata/fixtures/chat_translate/drops/common_drops.txt`](https://github.com/inja-online/llm-gateway/blob/master/testdata/fixtures/chat_translate/drops/common_drops.txt)
