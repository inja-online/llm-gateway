# Operator dashboard

**Last updated:** 2026-09-04

> **Docs site (recommended):** [Operator dashboard](https://inja-online.github.io/llm-gateway/guides/dashboard/) — profiles, usage, logs, edge auth, OAuth, sqlite, build tags.

Open [http://127.0.0.1:8787/ui/](http://127.0.0.1:8787/ui/) after the gateway is running. JSON/SSE lives at `/v1/dashboard`.

| | |
|---|---|
| SPA | `/ui/` — unauthenticated HTML/JS/CSS |
| API | `/v1/dashboard/*` — same `edge_auth` as `/v1/messages` |
| Default | `dashboard.enabled: true` when the key is omitted |
| Off | `dashboard.enabled: false` → proxy only |
| Edge key | SPA stores it in `sessionStorage` (`inja.edge_key`) |
| SQLite | `hooks.sqlite.path` opt-in; empty = off |
| Tags | `go build -tags noweb` (no `/ui`, API stays); `-tags nodb` (no sqlite) |

If `listen` is not loopback, enable `edge_auth`. Dashboard JSON never includes tokens (`access_token`, `refresh_token`, `client_secret`, raw API keys, request bodies).

**ChatGPT OAuth** uses the Codex public client redirect `http://localhost:1455/auth/callback` — not `/ui/oauth/callback`. The gateway exchanges the code; the SPA never sees `code` or the PKCE verifier.

Related: [README Dashboard](https://github.com/inja-online/llm-gateway/blob/master/README.md#dashboard) · [oauth-token-sources.md](oauth-token-sources.md)
