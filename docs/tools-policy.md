# Tools policy

**What:** how function tools and the expanded tool union behave on **passthrough** vs **translation**.

## Summary

| Path | Supported tools |
|------|-----------------|
| **Passthrough** (same family, e.g. OpenAI → openai / openai_compat) | **Everything** — JSON forwarded as-is after model rewrite |
| **Translate → OpenAI** | Full union rebuilt: `function`, `custom`, `computer`, `server` |
| **Translate → Anthropic / Google** | **`function` only** — other kinds → **error** (not silent skip) |

**Rationale:** silent skip made agents believe tools were registered when they were dropped.

---

## Tool kinds (canonical IR)

| Kind | Typical wire | IR fields | OpenAI egress | Anthropic/Google egress |
|------|--------------|-----------|---------------|-------------------------|
| `function` | `type: "function"` + `function.{name,parameters}` | Name, Schema, Description | yes | yes |
| `custom` | `type: "custom"` + grammar `format` | Grammar, GrammarType, Extra | yes | **error** |
| `computer` | computer-use styles | Name | yes | **error** |
| `server` | `file_search`, `web_search`, … | Name, Extra | yes | **error** |

Empty `type` is treated as **function**.

---

## How translation works

1. **OpenAI ingress** parses tools into IR (`parseTools`).  
2. Router picks upstream kind.  
3. **Egress** rebuilds vendor JSON — or returns a clear error for unsupported kinds on Anthropic/Google.

### Custom / grammar tools (OpenAI)

```json
{
  "type": "custom",
  "custom": {
    "name": "date_tool",
    "description": "Emit a date",
    "format": {
      "type": "regex",
      "definition": "[0-9]{4}-[0-9]{2}-[0-9]{2}"
    }
  }
}
```

Use **passthrough** to an OpenAI-family host, or translate **only** to another OpenAI-shaped provider.

### Function tools (all families)

```json
{
  "type": "function",
  "function": {
    "name": "lookup",
    "description": "Look up a record",
    "parameters": {
      "type": "object",
      "properties": { "id": { "type": "string" } },
      "required": ["id"]
    }
  }
}
```

Name is required. Schema is carried on the IR and rebuilt per dialect.

---

## Practical guidance

1. Need vendor-specific tools (file_search, custom grammar)? → **passthrough** to that vendor’s kind.  
2. Need OpenAI client → Claude/Gemini? → send **function tools only**.  
3. Tool **results** and multi-turn tool loops follow the same family rules as chat (signatures / `reasoning_content` / `thoughtSignature` preserved where IR supports them).  

---

## Related

- [Chat field parity](chat-field-parity.md)  
- [Compatibility matrix](compatibility-matrix.md)  
- [Deprecation policy](deprecation-policy.md)  
