# Operator dashboard — design

Date: 2026-09-04
Status: approved (product locks + architecture option 1). ChatGPT PKCE redirect is a recorded ruling, not a product change.

## Goal

Ship a React operator dashboard for Inja LLM Gateway: auth profiles (including multi-account pool), token usage, and request logs. Full control plane (list / disable / logout / OAuth login / import). Three artifacts from one tree:

1. Default static binary: SPA embedded, served at `/ui`, API at `/v1/dashboard`.
2. `-tags noweb`: no embed, no `/ui` (API stays so a separately hosted SPA can talk to the gateway).
3. Web-only: Vite `web/dist` as a GitHub Release zip (and folder). Same SPA. Optional `dashboard.cors_origin` when not same-origin.

Compose `-tags nodb` to strip SQLite (`modernc.org/sqlite`). Smallest binary: `-tags noweb,nodb`.

## Non-goals (v1)

- Database for sessions, IAM, or multi-tenant users.
- Request/response bodies or raw tokens in any JSON, log, or UI.
- CORS wide-open (`*`).
- EventSource (cannot send `Authorization`). SSE uses `fetch` + `ReadableStream`.
- Changing Codex/ChatGPT OAuth client id or redirect URI.
- Dashboard inside `gateway.New` (library stays proxy-only).

## Constraints (existing project)

- `CGO_ENABLED=0` static binary, distroless Docker, Go 1.25.
- Coverage floors: overall ≥85%, core (`config/proxy/hooks/ingress/egress`) ≥89%.
- No live provider calls in CI. `httptest` only.
- Never log or JSON-encode `access_token`, `refresh_token`, `client_secret`.
- One `hooks.UsageEvent` per proxied request. Ring and SQLite are extra `hooks.Hook`s, not a second metering path.
- `proxy/` stays dialect/proxy. Dashboard is binary-only.

## Architecture

```
browser ──► cmd/gateway root mux
              ├─ /ui/*                 embed.FS (exempt from edge_auth)
              ├─ /v1/dashboard/*       dashboard.Handler (edge_auth like /v1/messages)
              └─ /                     proxy.Handler() (existing edge_auth + access log)
```

`gateway.New` unchanged in role: YAML jsonl/webhook + `WithHook`. The binary passes ring + sqlite slot via `WithHook` when the dashboard is enabled.

```
dashboard/          HTTP JSON + SSE (operator API)
hooks/ring/         last-N UsageEvent
hooks/sqlite/       optional durable events (!nodb / nodb stub)
web/                Vite + React + TypeScript SPA
cmd/gateway/dist/   copied from web/dist at CI/release; stub committed for `go test`
cmd/gateway/ui.go   //go:embed all:dist  (!noweb)
cmd/gateway/ui_noweb.go
cmd/gateway/operator.go  mount + hook wiring
```

## Config

All optional. `KnownFields` stays on.

```yaml
dashboard:
  enabled: true          # false: no /ui, no /v1/dashboard (binary still a proxy)
  ring_size: 2000        # 1..100000; default 2000; 0 → 2000
  cors_origin: ""        # empty: no CORS. Else exact origin for web-only SPA

hooks:
  jsonl: { output: stdout }
  sqlite:
    path: ""             # empty = off. File created 0600 on first write.
```

Defaults when the `dashboard:` key is omitted: `enabled: true`, `ring_size: 2000`, `cors_origin: ""`.

`-tags noweb` does not change YAML; it omits `/ui` only.

## Auth / edge_auth

- `/ui/` is always unauthenticated (HTML/JS/CSS). Same pattern as `/healthz`.
- `/v1/dashboard/*` uses the same `edge_auth` keys as `/v1/messages` (`Authorization: Bearer` or `x-api-key`, constant-time). When `edge_auth.enabled` is false, the API is open (laptop default).
- Export a package-level wrapper from `proxy` so dashboard does not copy the compare loop: `proxy.WrapEdgeAuth(cfg *config.Config, h http.Handler) http.Handler`. Exempt paths stay `/healthz` and `/metrics` on the proxy mux. Dashboard handler is wrapped as a whole (no extra exempt paths).
- SPA: on 401, show a gate, store the edge key in `sessionStorage` (`inja.edge_key`), send `Authorization` on every API call. Do not use `localStorage` (shared with XSS on the origin for longer).
- SSE: `GET /v1/dashboard/logs/stream` with `Authorization` via `fetch`, not `EventSource`.

## Secrets policy

Dashboard JSON, SSE, and HTML never include: `access_token`, `refresh_token`, `client_secret`, raw API keys, request bodies.

Profile DTO:

```json
{
  "provider": "chatgpt",
  "account_id": "",
  "source": "oauth_pkce",
  "expiry": "2026-09-04T12:00:00Z",
  "usable": true,
  "has_refresh": true,
  "has_access": true,
  "access_state": "present",
  "cooldown_until": null,
  "disabled": false,
  "updated_at": "2026-09-04T11:00:00Z"
}
```

`access_state` is `missing` | `empty` | `present` | `expired` (same meaning as `llm-gateway auth status`).

## HTTP API

OpenAI-shaped errors (`{"error":{"message","type","code"}}`). Content-Type `application/json` except SSE (`text/event-stream`).

| Method | Path | Body / query | Result |
|---|---|---|---|
| GET | `/v1/dashboard/meta` | | `{version, dashboard: true, sqlite: bool, noweb: false, nodb: bool, ring_size, sqlite_path_set}` — `sqlite` is “this binary was built with sqlite”; `sqlite_path_set` is whether a path is active |
| GET | `/v1/dashboard/profiles` | | `{profiles: [ProfileDTO, ...]}` primary + pool, all known providers (missing → `access_state: missing`) |
| POST | `/v1/dashboard/profiles/{provider}/logout` | query `account` optional | 204. Empty account → primary. `all` as provider → all providers |
| POST | `/v1/dashboard/profiles/{provider}/disable` | `{disabled: bool, account?: string}` | 204 |
| POST | `/v1/dashboard/oauth/start` | `{provider}` | see OAuth |
| POST | `/v1/dashboard/oauth/complete` | `{provider, token?}` | Claude paste. 204 |
| POST | `/v1/dashboard/oauth/import` | `{provider}` | server reads well-known CLI file. 204 |
| GET | `/v1/dashboard/oauth/status?provider=` | | `{state: pending\|complete\|error\|idle, kind, authorize_url?, user_code?, verification_uri?, error?}` |
| GET | `/v1/dashboard/usage` | `from`, `to` RFC3339, `provider`, `model` | `{totals: {tokens_in, tokens_out, requests, by_provider, by_model, by_status}, recent: [UsageEvent]}` |
| GET | `/v1/dashboard/logs` | `cursor`, `limit` (default 100, max 500), filters `provider`, `model`, `status`, `q` (request_id substring) | `{events, next_cursor}` newest first |
| GET | `/v1/dashboard/logs/stream` | last-event via SSE comment/retry | `event: usage` data = one UsageEvent JSON. Heartbeat comment every 15s |
| GET | `/v1/dashboard/settings` | | `{ring_size, sqlite_path, sqlite_enabled, jsonl_output}` |
| PUT | `/v1/dashboard/settings` | `{sqlite_path?: string, ring_size?: int}` | applies process-local (sqlite slot swap, ring resize). Does not rewrite `gateway.yaml` |

`UsageEvent` in JSON is the existing `hooks.UsageEvent` struct (no new fields).

Unknown provider on path → 404. Bad JSON → 400.

## OAuth (full control plane)

Reuse `subauth`. Do not invent a fifth dance.

### ChatGPT — ruling vs earlier sketch

Codex public client (`app_EMoamEEZ73f0CkXaXp7hrann`) redirects to **`http://localhost:1455/auth/callback`** (CLI already binds this). A gateway `/ui/oauth/callback` URL will not be accepted by the IdP.

**v1 behavior:** `POST /oauth/start` for `chatgpt` starts the existing loopback listener (`subauth` extracted start/complete), returns `{kind:"redirect", authorize_url, state}`. The SPA opens `authorize_url` (same tab or `window.open`). OpenAI returns to `localhost:1455`. The **gateway** exchanges the code (SPA never sees `code` or PKCE verifier). SPA polls `GET /oauth/status?provider=chatgpt` until `complete` or `error`. Pending session TTL 10 minutes, RAM only.

Extract from `subauth.LoginChatGPT`:

- `StartChatGPT(ctx, noBrowser bool) (authorizeURL, redirectURI, pkce, state, listener, error)` — or a small `ChatGPTSession` type the CLI and dashboard both use.
- CLI `LoginChatGPT` keeps the same user-visible behavior.

### Grok

`POST /oauth/start` `{provider:"grok"}` begins device-code (always device; do not auto-import on start — import is a separate button). Returns `{kind:"device", user_code, verification_uri, expires_in, interval}`. Gateway polls the token endpoint in a goroutine. SPA polls `/oauth/status`. Prefer `verification_uri` (not the prefilled complete URL), matching CLI comments.

### Claude

No start. `POST /oauth/complete` `{provider:"claude", token:"sk-ant-oat…"}`. Empty token → 400.

### Gemini

No browser login. Import-only + optional paste of the Antigravity JSON object via `complete` `{provider:"gemini", token:"<json or refresh token>"}` if cheap; otherwise import-only is enough. Prefer import-only in v1 (paste JSON is extra). **v1: import only** for Gemini.

### Import

`POST /oauth/import` `{provider}` calls existing `Import*FromCLI` on the **server** filesystem (not an upload). 400 if file missing.

## Usage data

Priority when serving `/usage` and `/logs`:

1. SQLite if a path is active (filter in SQL).
2. Else if `hooks.jsonl.output` is a filesystem path (not `stdout`/`stderr`/empty), tail-scan the file (last N matching; cap 2 MiB read from end).
3. Else the in-process ring.

Ring always receives `OnUsage` when dashboard is enabled, even if sqlite/jsonl exist (live tail without file I/O).

Ring: mutex + ring buffer, default 2000, drop oldest. `Snapshot() []hooks.UsageEvent` oldest→newest copy.

SQLite (`modernc.org/sqlite`, CGO-free):

```sql
CREATE TABLE IF NOT EXISTS usage_events (
  request_id TEXT PRIMARY KEY,
  time TEXT NOT NULL,
  dialect_in TEXT,
  provider TEXT,
  model TEXT,
  upstream_model TEXT,
  modality TEXT,
  transport TEXT,
  tokens_in INTEGER,
  tokens_out INTEGER,
  cached_tokens INTEGER,
  cache_write_tokens INTEGER,
  reasoning_tokens INTEGER,
  estimated INTEGER,
  stream INTEGER,
  status TEXT,
  http_status INTEGER,
  latency_ms INTEGER,
  ttft_ms INTEGER,
  key_hash TEXT,
  dropped_fields TEXT,
  media TEXT
);
CREATE INDEX IF NOT EXISTS idx_usage_time ON usage_events(time);
```

WAL on. Inserts must not block the proxy: if the write lock is busy, drop the event (ponytail: global mutex, WAL if throughput matters). Tests cover insert + query.

Settings PUT `sqlite_path`: empty string closes the slot (off). Non-empty opens/creates the file 0600. Slot is a `hooks.Hook` that no-ops when closed so `hooks.Multi` need not be rebuilt.

`-tags nodb`: `hooks/sqlite` is a stub (`New` returns an error or a no-op hook; `Available() bool` is false). Settings PUT sqlite_path → 400 `sqlite_disabled`.

## SPA (`web/`)

Vite 6/7 + React 19 + TypeScript. **No UI component library. No chart library.** CSS in `web/src/styles.css`. Charts: inline SVG (bar/line from the usage totals + recent points).

`base: '/ui/'`. React Router basename `/ui`. Fallback: Go file server serves `index.html` for `/ui/*` without a file extension.

Pages:

- **Profiles** (`/ui/`): table of providers + pool accounts. Actions: Login (chatgpt/grok), Paste token (claude), Import, Disable, Logout. Status badges from `access_state`.
- **Usage** (`/ui/usage`): KPI tiles (requests, tokens in, tokens out) + simple SVG breakdown by provider/model. Table of recent events.
- **Logs** (`/ui/logs`): same events, filters, live tail via fetch-SSE (fallback poll 2s if stream errors).
- **Settings** (`/ui/settings`): show listen/version, toggle sqlite path (text field), ring size (read-only unless we allow PUT — PUT is allowed). Warning that PUT does not rewrite YAML.

Gate overlay when API returns 401.

Web-only build: `VITE_GATEWAY_ORIGIN` empty means same-origin (`''`). If set, fetch that origin (and CORS must be configured on the gateway).

## Embed / CI / release

- Commit `cmd/gateway/dist/index.html` stub so `go test ./cmd/gateway` works without npm (`Dashboard not built — npm run build in web/`).
- CI `test` job: unchanged `go test` (stub UI).
- CI `smoke-binary`: after `go build`, `curl /ui/` returns 200 HTML.
- Release: `npm ci && npm run build` in `web/`, copy `web/dist/*` → `cmd/gateway/dist/`, then the existing GOOS/GOARCH matrix. Also attach `dist/inja-gateway-ui_${version}.zip` (the SPA).
- Extra release binaries optional: `llm-gateway_${ver}_${os}_${arch}_noweb` is **not** required in v1 if tags are documented. v1: default matrix embeds UI. Document `go build -tags noweb` / `-tags nodb` in README.
- Docker: default image embeds UI (needs Node in build stage). Multi-stage: node build web, then golang build.

## Testing

- `config`: parse dashboard + sqlite, defaults, reject `ring_size` > 100000.
- `hooks/ring`: wrap, snapshot order, overflow drops oldest.
- `hooks/sqlite`: tempfile insert/query; `nodb` stub compile test via `go test -tags nodb`.
- `dashboard`: httptest, fake store path in t.TempDir, never assert token values in body (assert they are absent). OAuth chatgpt: fake loopback + fake token URL if extracted; otherwise test complete/import/disable/logout/profiles/logs/usage against a seeded ring.
- `cmd/gateway`: `/ui/` 200 with stub; `-tags noweb` build smoke in CI optional (one `go build -tags noweb`).
- No live OpenAI/xAI.

## Docs

- README: dashboard section, tags, security (`edge_auth` on when listen is not loopback).
- `gateway.example.yaml` comments.
- CHANGELOG `[Unreleased]`.
- CONTRIBUTING: “No database” → “No required database; optional CGO-free SQLite usage log (`hooks.sqlite.path`, stripped with `-tags nodb`)”.
- `website/src/content/docs/` mirror of the operator-facing bits.

## Security notes

- PKCE verifier and OAuth `code` never sent to the browser.
- `sessionStorage` for edge key (tab-scoped).
- SQLite and credentials file stay 0600.
- Access log must not grow to include Authorization headers (already does not).
- Dashboard disable/logout is a local-store mutation, not an upstream revoke.
