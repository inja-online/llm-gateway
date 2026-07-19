# Contributing

Thanks for helping with **Inja LLM Gateway** (`llm-gateway`). Keep the bar where it is: small, clear, and boring to operate.

## Principles

- Prefer **passthrough** over translation when dialects match.
- One usage event per chat/media request — never drop error/abort paths.
- No database; optional edge auth is a static key gate only (not a full IAM product).
- Simplicity over framework: stdlib + `yaml.v3` when possible.
- **No live provider calls in CI.** Air-gapped tests with `httptest` / fakes only.
- Never log request bodies or raw API keys; usage events may include `key_hash` only.
- Do **not** invent a fake Anthropic Realtime WebSocket dialect.

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
- [ ] [`CHANGELOG.md`](CHANGELOG.md) entry under `[Unreleased]` when the public HTTP/WS/config surface changes
- [ ] Compatibility matrix / deprecation notes if modality or drop behavior changes
- [ ] `go vet` / race clean
- [ ] No secrets in commits (use `api_key_env` + env; edge keys via `keys_env`)

## Layout

| Package | Role |
|---|---|
| `proxy/` | HTTP pipeline (OpenAI, Anthropic, Google handlers; media; edge auth) |
| `proxy/token.go` | `TokenSource` for ADC / service-account style Bearer tokens |
| `canonical/` | Dialect-neutral IR (same package). Files by modality: `chat.go`, `image.go`, `video.go`, `audio.go`, `realtime.go` — do not re-merge into one file |
| `ingress/openai` | OpenAI dialect (+ Gemini OpenAI-compat clients) |
| `ingress/anthropic` | Anthropic Messages dialect |
| `ingress/google` | Gemini **native** `generateContent` dialect |
| `egress/openai` | OpenAI / `openai_compat` upstream (incl. Gemini OpenAI-compat) |
| `egress/anthropic` | Anthropic upstream |
| `egress/google` | Gemini **native** upstream (`kind: google`) |
| `hooks/*` | Usage sinks |
| `config/` | YAML + capabilities + edge_auth |
| `internal/sse/` | SSE helpers |
| `internal/testutil/` | Shared test helpers (as needed) |
| `testdata/fixtures/` | Golden media/wire samples |
| `cmd/gateway` | Binary |
| `docs/` | Compatibility matrix, checklists, policies |

Gemini has two Google APIs: native (`kind: google` + `ingress/google`) and OpenAI-compat (`kind: openai_compat` + OpenAI ingress/egress).

## Adding a modality

Follow the capability-centric design (`docs/superpowers/specs/2026-07-18-multimodal-gateway-design.md` §12). Checklist (Definition of Done spirit):

1. **Capability flag** — add or reuse `config.Capabilities` field; set **kind defaults** (`openai`/`google` often on; `openai_compat` **opt-in**; `anthropic` off unless native API exists).
2. **Passthrough vs translate** — same family → passthrough (model rewrite + auth + one usage event). Cross family → canonical types + ingress/egress builders.
3. **Ingress routes** — dialect-shaped paths/headers/errors (OpenAI vs Anthropic `anthropic-version` disambiguation vs Google `:method`).
4. **Egress** — only call upstream when `Supports(modality)`; else fail closed **before** network.
5. **Hooks** — exactly one `UsageEvent` with `modality` / `media` fields as designed; no body/key logging.
6. **Config** — document fields in README + `gateway.example.yaml` comments.
7. **Tests** — matrix cells (dialect × kind → 200 or 4xx) in `proxy/capability_matrix_test.go`; fake upstream `t.Fatal` on deny cells; race-clean; **no** `time.Sleep`; **no** live network. Add a row when a modality lands.
8. **Fixtures** — `testdata/fixtures/media/{dialect}_{modality}_{case}.json` with tiny base64 (`YQ==`); see [`testdata/fixtures/media/README.md`](testdata/fixtures/media/README.md).
9. **Docs** — README API table, [docs/compatibility-matrix.md](docs/compatibility-matrix.md), [CHANGELOG.md](CHANGELOG.md); drop lists per [docs/deprecation-policy.md](docs/deprecation-policy.md).
10. **Canonical types** — put new IR types in the matching modality file under `canonical/` (`chat.go` / `image.go` / `video.go` / `audio.go` / `realtime.go`); keep package name `canonical`.

### Ban list

- Live network tests in default CI (`-tags live` only if optional and documented)
- Inventing Anthropic Realtime WS
- Logging bodies, multipart filenames as secrets, or raw keys
- Half-bridging realtime protocols without explicit drop lists / errors
- Silent capability allow when `openai_compat` defaults media off

### Example issue themes

- Image gen: OpenAI `/v1/images/*` passthrough already exists; Anthropic/Google media contracts and cross-dialect IR are separate issues
- Realtime: OpenAI RT + Google Live passthrough before any bridge

## Release

Maintainers only:

```bash
# Ensure checklist: tests, changelog, Claude Code doc if Anthropic touched
git tag vX.Y.Z
git push origin vX.Y.Z
```

GitHub Actions builds multi-arch binaries and attaches them to the release.

Claude Code sign-off: [docs/claude-code-checklist.md](docs/claude-code-checklist.md).

## Related docs

| Doc | Purpose |
|---|---|
| [CHANGELOG.md](CHANGELOG.md) | Versioning + release notes |
| [docs/compatibility-matrix.md](docs/compatibility-matrix.md) | Dialect × modality × kind |
| [docs/deprecation-policy.md](docs/deprecation-policy.md) | Field drops / semver |
| [docs/claude-code-checklist.md](docs/claude-code-checklist.md) | Anthropic client regression |
| [SECURITY.md](SECURITY.md) | Reporting + deploy auth notes |
