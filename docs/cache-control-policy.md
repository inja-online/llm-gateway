# Prompt caching policy (#108)

**Last updated:** 2026-07-21  
**Status:** Cross-dialect IR implemented (preserve within family; drop cross-family)

## Summary

| Dialect | Request-side caching | On translate |
|---|---|---|
| **Anthropic** | `cache_control` on system / content / tools | **Preserved** on Anthropic egress (PT + IR rebuild). **Dropped** toward OpenAI/Google. |
| **OpenAI** | `prompt_cache_key`, `prompt_cache_retention` | **Preserved** on OpenAI egress. **Dropped** toward Anthropic/Google (no auto breakpoints). |
| **Google** | `cachedContent` / `cached_content` resource name | **Preserved** on Google egress. **Dropped** toward others. |

**Usage (response):** `CacheReadTokens` / `CacheWriteTokens` continue to map from all three vendors when present.

## Supersedes #41 Option B

#41 documented **passthrough-only** (strip on any IR rebuild). **#108 supersedes** that for Anthropic→Anthropic **translate**: breakpoints are modeled in canonical IR and rebuilt.

Passthrough Anthropic→Anthropic still never touches IR (byte path).

## Canonical IR

- `canonical.CacheControl` on `Block` and `Tool`
- `Request.PromptCacheKey`, `Request.PromptCacheRetention`
- `Request.CachedContent`

## Non-goals (still)

- Gateway-local prompt cache store
- Guaranteeing cache hits
- Auto-insert breakpoints without client intent
- Full Google `cachedContents` CRUD (see #112)

## Tests

- `ingress/anthropic.TestCacheControlParseAndRoundTrip`
- `ingress/openai.TestPromptCacheKeyRoundTrip`
- `ingress/google.TestCachedContentRoundTrip`
- `proxy.TestChatTranslateFixtures` (`anthropic_to_anthropic_preserves_cache_control`)
- `proxy.TestCacheControlPassthroughAnthropic` (PT regression)

## Related

- [deprecation-policy.md](deprecation-policy.md)
- [compatibility-matrix.md](compatibility-matrix.md)
