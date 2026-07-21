# Prompt caching policy (#108)

**Last updated:** 2026-07-21  
**Status:** Cross-dialect IR implemented (preserve within family; drop cross-family) + opt-in Anthropic auto breakpoints

## Summary

| Dialect | Request-side caching | On translate |
|---|---|---|
| **Anthropic** | `cache_control` on system / content / tools | **Preserved** on Anthropic egress (PT + IR rebuild). **Dropped** toward OpenAI/Google. |
| **OpenAI** | `prompt_cache_key`, `prompt_cache_retention` | **Preserved** on OpenAI egress. **Dropped** toward Anthropic/Google. |
| **Google** | `cachedContent` / `cached_content` resource name | **Preserved** on Google egress. **Dropped** toward others. Create/list/update/delete via `/v1beta/cachedContents*` (kind:google). |

**Usage (response):** `CacheReadTokens` / `CacheWriteTokens` continue to map from all three vendors when present.

## Supersedes #41 Option B

#41 documented **passthrough-only** (strip on any IR rebuild). **#108 supersedes** that for Anthropic→Anthropic **translate**: breakpoints are modeled in canonical IR and rebuilt.

Passthrough Anthropic→Anthropic still never touches IR (byte path).

## Canonical IR

- `canonical.CacheControl` on `Block` and `Tool`
- `Request.PromptCacheKey`, `Request.PromptCacheRetention`
- `Request.CachedContent`

## Opt-in Anthropic auto breakpoints

When clients speak OpenAI or Google but the upstream is Anthropic, they often have no way to express `cache_control`. Operators may enable **optional** injection:

```yaml
caching:
  auto_breakpoints:
    enabled: true          # default false — never invent breakpoints without opt-in
    min_chars: 2048        # default 2048; per-target total text length threshold
    targets: [system, tools]  # default both when empty; only system|tools
```

**Behavior (v1):**

| Rule | Detail |
|---|---|
| Scope | Translate paths that **rebuild toward Anthropic only** (`/v1/chat/completions` → Anthropic, Google generateContent → Anthropic). **Not** Anthropic passthrough. |
| Placement | Last system **text** block and/or **last** tool def → `cache_control: {type: ephemeral}` |
| Client wins | If any `cache_control` already exists on that surface (system blocks / tools), skip that target |
| Threshold | Sum of system text lengths / tool name+description+schema must be ≥ `min_chars` |
| Observability | Response header `X-Gateway-Cache-Auto: system,tools` lists targets that were applied |
| Non-goals | Does **not** auto-create Google `cachedContents`; does **not** invent OpenAI `prompt_cache_*` |

## Non-goals (still)

- Gateway-local prompt cache store
- Guaranteeing cache hits
- Auto-insert breakpoints when config is off (default)
- Auto-create Google caches on chat (stateless edge; use `/v1beta/cachedContents` explicitly)

## Tests

- `ingress/anthropic.TestCacheControlParseAndRoundTrip`
- `ingress/openai.TestPromptCacheKeyRoundTrip`
- `ingress/google.TestCachedContentRoundTrip`
- `proxy.TestChatTranslateFixtures` (`anthropic_to_anthropic_preserves_cache_control`)
- `proxy.TestCacheControlPassthroughAnthropic` (PT regression)
- `proxy.TestApplyAutoBreakpoints*` / `TestOpenAIToAnthropicAutoBreakpoints*`
- `config.TestParseCachingAutoBreakpoints`

## Related

- [deprecation-policy.md](deprecation-policy.md)
- [compatibility-matrix.md](compatibility-matrix.md)
