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
| `proxy/` | HTTP pipeline |
| `ingress/*` | Client dialect → canonical |
| `egress/*` | Canonical → upstream |
| `hooks/*` | Usage sinks |
| `cmd/gateway` | Binary |

## Release

Maintainers only:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

GitHub Actions builds multi-arch binaries and attaches them to the release.
