# Chat translation golden fixtures

Air-gapped fixtures that lock **which fields are preserved vs intentionally dropped**
on cross-dialect chat translation. CI runs these with `go test` (no network).

## Layout

```
chat_translate/
  README.md                 # this file
  drops/
    common_drops.txt        # fields currently dropped on every translate path
  openai/
    kitchen_sink.json       # OpenAI chat-completions request (ingress)
  anthropic/
    kitchen_sink.json       # Anthropic Messages request (ingress)
  google/
    kitchen_sink.json       # Gemini generateContent request (ingress)
```

## How fixtures are produced

1. Hand-authored offline from public provider schema docs (no live recordings).
2. When a fidelity issue lands, update the matching kitchen-sink field **and**
   remove it from `drops/common_drops.txt` (or dialect-pair drop list) in the same PR.
3. Prefer normalized JSON key presence checks over raw byte equality.

## Policies locked by these fixtures

| Field / feature | Policy |
|---|---|
| `n` / `candidateCount` | Translate supports **n=1 only**; `n>1` is `bad_request` |
| Non-function OpenAI tools | **error** (not silent skip) |
| Anthropic `cache_control` | **Passthrough-only** — stripped on translate rebuild |
| Google `safetySettings` | Preserved on Google→Google translate; dropped for other ingress |
| `logprobs`, `seed`, `response_format`, penalties | Dropped until dedicated fidelity issues land |
| `service_tier` / `system_fingerprint` | Optional OpenAI-only metadata when present |

## Running

```bash
go test ./proxy/ -run TestChatTranslateFixtures -count=1
```
