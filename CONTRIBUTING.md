# Contributing

Thanks for helping with **Inja LLM Gateway** (`llm-gateway`). Keep the bar where it is: small, clear, and boring to operate.

## Principles

- Prefer **passthrough** over translation when dialects match.
- One usage event per chat request — never drop error/abort paths.
- No database or auth layer unless the design explicitly changes.
- Simplicity over framework: stdlib + `yaml.v3` when possible.

## Dev loop

```bash
go test ./...
go test -race ./...
go vet ./...
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1
```

Coverage gate in CI is **≥ 90%**. Add tests with behavior changes.

## PR checklist

- [ ] Tests for new paths (unit and/or smoke under `proxy/`)
- [ ] README updated if the public surface or deploy story changes
- [ ] `go vet` / race clean
- [ ] No secrets in commits (use `api_key_env` + env)

## Layout

| Package | Role |
|---|---|
| `proxy/` | HTTP pipeline (OpenAI, Anthropic, Google native handlers) |
| `ingress/openai` | OpenAI dialect (+ Gemini OpenAI-compat clients) |
| `ingress/anthropic` | Anthropic Messages dialect |
| `ingress/google` | Gemini **native** `generateContent` dialect |
| `egress/openai` | OpenAI / `openai_compat` upstream (incl. Gemini OpenAI-compat) |
| `egress/anthropic` | Anthropic upstream |
| `egress/google` | Gemini **native** upstream (`kind: google`) |
| `hooks/*` | Usage sinks |
| `cmd/gateway` | Binary |

Gemini has two Google APIs: native (`kind: google` + `ingress/google`) and OpenAI-compat (`kind: openai_compat` + OpenAI ingress/egress).

## Release

Maintainers only:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

GitHub Actions builds multi-arch binaries and attaches them to the release.
