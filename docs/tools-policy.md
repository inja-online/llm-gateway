# Tools policy (#49, #107, #161)

**Last updated:** 2026-07-22

## Policy

| Path | Tool kinds |
|---|---|
| **Passthrough** (OpenAI → `openai` / `openai_compat`) | **Forward as-is** (JSON map; model rewrite only). |
| **Translation → OpenAI** | Full tool union rebuilt: `function`, `custom` (grammar), `computer`, `server`. |
| **Translation → Anthropic / Google** | **Only `function`**. Other kinds → build error (fail closed, not silent skip). |

**Rationale:** silent skip made agents believe tools were registered when they were not. Custom/grammar tools are first-class on OpenAI family; Anthropic/Google have no equivalent wire shape.

## Canonical tool union

| Kind | Wire | IR fields |
|------|------|-----------|
| `function` | `type: function` + `function.{name,description,parameters}` | Name, Schema |
| `custom` | `type: custom` + grammar `format` | Grammar, GrammarType, Extra |
| `computer` | computer-use styles | Name |
| `server` | `file_search`, `web_search`, … | Name, Extra |

## Function tools

Name required, parameters JSON schema on IR, rebuilt per dialect.

## Related

- [#107](https://github.com/inja-online/llm-gateway/issues/107) tool union
- [#161](https://github.com/inja-online/llm-gateway/issues/161) custom grammar tools
