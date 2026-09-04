# Operator Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a React operator dashboard (profiles, usage, logs, OAuth) inside the llm-gateway binary, with `-tags noweb` / `-tags nodb` and a separate SPA zip.

**Architecture:** `cmd/gateway` root mux mounts `/ui/` (embed) and `/v1/dashboard/` (`dashboard` package). `proxy/` stays dialect-only. Ring + SQLite are extra `hooks.Hook`s wired via `gateway.WithHook`. Library `gateway.New` does not serve the UI.

**Tech Stack:** Go 1.25, stdlib net/http, `modernc.org/sqlite` (CGO-free, `-tags nodb` stub), Vite + React 19 + TypeScript (no UI/chart libraries).

**Spec:** `docs/superpowers/specs/2026-09-04-operator-dashboard-design.md`

## Global Constraints

- `CGO_ENABLED=0`; never add CGO sqlite.
- Coverage floors stay: overall ≥85%, core (`config/proxy/hooks/ingress/egress`) ≥89%.
- No live provider HTTP in CI (`httptest` fakes only).
- Dashboard JSON/SSE/HTML never include `access_token`, `refresh_token`, `client_secret`, raw API keys, or request bodies.
- One `hooks.UsageEvent` per proxied request. Ring and SQLite implement `hooks.Hook`; do not add a second emit path in `proxy/exchange.go`.
- `proxy/` does not register `/ui` or `/v1/dashboard`.
- `gateway.New` stays proxy-only; operator surface is `cmd/gateway`.
- ChatGPT OAuth redirect URI remains `http://localhost:1455/auth/callback` (Codex client). SPA never receives `code` or PKCE verifier.
- `/ui/` is exempt from `edge_auth`. `/v1/dashboard/*` uses the same keys as `/v1/messages`.
- `dashboard.enabled` defaults **true** (`*bool`; nil → true). `enabled: false` opts out.
- `ring_size` 0 or omitted → 2000; max 100000.
- `sessionStorage` key for the edge key is `inja.edge_key` (not localStorage).
- SSE via `fetch` + stream, not `EventSource`.
- Commit messages: `feat:` / `fix:` / `chore:` / `docs:` / `test:`. End with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.
- Work from this worktree only. Do not push. Do not spawn subagents. TDD: failing test first.
- Never log secrets. Tests that handle credentials must `t.Setenv("INJA_GATEWAY_AUTH_FILE", temp)` and assert response bodies do **not** contain the token strings.

---

### Task 1: Config — dashboard + sqlite YAML

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`
- Modify: `gateway.example.yaml` (comment block only if it compiles without new keys — add commented `dashboard:` and `hooks.sqlite`)

**Interfaces:**
- Consumes: existing `Config`, `Hooks`, `Parse`, `KnownFields(true)`
- Produces:
  - `type Dashboard struct { Enabled *bool `yaml:"enabled"`; RingSize int `yaml:"ring_size"`; CORSOrigin string `yaml:"cors_origin"` }`
  - `func (d Dashboard) IsEnabled() bool` — nil or omitted → true
  - `func (d Dashboard) Ring() int` — ≤0 → 2000; >100000 invalid at validate
  - `type SQLiteHook struct { Path string `yaml:"path"` }`
  - `Hooks.SQLite *SQLiteHook `yaml:"sqlite"``
  - `Config.Dashboard Dashboard `yaml:"dashboard"``
  - `const DefaultDashboardRingSize = 2000`
  - `const MaxDashboardRingSize = 100000`

- [ ] **Step 1: Write failing tests** in `config/config_test.go`:

```go
func TestDashboardDefaultsEnabled(t *testing.T) {
	cfg, err := Parse([]byte(`providers:
  x: { kind: openai, base_url: "https://x" }`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Dashboard.IsEnabled() {
		t.Fatal("dashboard should default enabled")
	}
	if cfg.Dashboard.Ring() != DefaultDashboardRingSize {
		t.Fatalf("ring = %d", cfg.Dashboard.Ring())
	}
}

func TestDashboardEnabledFalse(t *testing.T) {
	cfg, err := Parse([]byte(`providers:
  x: { kind: openai, base_url: "https://x" }
dashboard:
  enabled: false
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dashboard.IsEnabled() {
		t.Fatal("want disabled")
	}
}

func TestDashboardRingSizeReject(t *testing.T) {
	_, err := Parse([]byte(`providers:
  x: { kind: openai, base_url: "https://x" }
dashboard:
  ring_size: 100001
`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHooksSQLitePath(t *testing.T) {
	cfg, err := Parse([]byte(`providers:
  x: { kind: openai, base_url: "https://x" }
hooks:
  sqlite: { path: "/tmp/usage.sqlite" }
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks.SQLite == nil || cfg.Hooks.SQLite.Path != "/tmp/usage.sqlite" {
		t.Fatalf("sqlite = %#v", cfg.Hooks.SQLite)
	}
}
```

- [ ] **Step 2: Run** `go test ./config/ -count=1 -run 'TestDashboard|TestHooksSQLite'` — expect FAIL (unknown field / undefined).

- [ ] **Step 3: Implement** structs, `IsEnabled`, `Ring`, validate `ring_size` in `validate()`. Do not change `KnownFields`.

- [ ] **Step 4: Re-run tests** — PASS. `go test ./config/ -count=1`.

- [ ] **Step 5: Commit** `test: dashboard and sqlite config` then `feat: parse dashboard and hooks.sqlite` (or one commit if you prefer a single `feat: dashboard yaml config`).

---

### Task 2: Export `proxy.WrapEdgeAuth`

**Files:**
- Modify: `proxy/edge_auth.go`
- Modify: `proxy/server.go` (`Handler` still calls wrap)
- Test: `proxy/edge_auth_test.go` (add one test that WrapEdgeAuth on a standalone mux 401s)

**Interfaces:**
- Produces: `func WrapEdgeAuth(cfg *config.Config, h http.Handler) http.Handler`
- Behavior identical to today's `withEdgeAuth`: if cfg nil or `!EdgeAuth.Enabled`, return h; exempt `/healthz` and `/metrics`; Bearer / x-api-key constant-time.
- `(s *Server) withEdgeAuth` becomes `return WrapEdgeAuth(s.cfg, h)`.

- [ ] **Step 1: Failing test** — call `WrapEdgeAuth` with enabled keys wrapping a 200 handler; request without key → 401; with key → 200; `/healthz` without key → 200.

- [ ] **Step 2: Run** `go test ./proxy/ -count=1 -run EdgeAuth` — FAIL if symbol missing.

- [ ] **Step 3: Extract function.** Do not change error JSON.

- [ ] **Step 4:** `go test ./proxy/ -count=1 -run EdgeAuth` PASS.

- [ ] **Step 5: Commit** `feat: export proxy.WrapEdgeAuth for dashboard mux`

---

### Task 3: `hooks/ring` last-N buffer

**Files:**
- Create: `hooks/ring/ring.go`
- Create: `hooks/ring/ring_test.go`

**Interfaces:**
- Produces:
  - `type Sink struct` implementing `hooks.Hook`
  - `func New(size int) *Sink` — size≤0 → 2000
  - `func (s *Sink) OnUsage(ctx context.Context, ev hooks.UsageEvent)`
  - `func (s *Sink) Snapshot() []hooks.UsageEvent` — oldest→newest copy
  - `func (s *Sink) Resize(n int)` — drop oldest if shrinking
  - `func (s *Sink) Len() int`

- [ ] **Step 1: Failing test**

```go
func TestRingOverflowDropsOldest(t *testing.T) {
	s := New(2)
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "a"})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "b"})
	s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "c"})
	got := s.Snapshot()
	if len(got) != 2 || got[0].RequestID != "b" || got[1].RequestID != "c" {
		t.Fatalf("%+v", got)
	}
}

func TestRingConcurrentSnapshot(t *testing.T) {
	s := New(32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			s.OnUsage(context.Background(), hooks.UsageEvent{RequestID: "x"})
		}
	}()
	for i := 0; i < 100; i++ {
		_ = s.Snapshot()
	}
	<-done
}
```

- [ ] **Step 2:** `go test ./hooks/ring/ -count=1` FAIL.

- [ ] **Step 3:** Mutex + slice or ring indices. `OnUsage` must not block on IO.

- [ ] **Step 4:** PASS including `-race`.

- [ ] **Step 5: Commit** `feat: in-process usage event ring`

---

### Task 4: `hooks/sqlite` + `nodb` stub

**Files:**
- Create: `hooks/sqlite/sqlite.go` (`//go:build !nodb`)
- Create: `hooks/sqlite/sqlite_test.go` (`//go:build !nodb`)
- Create: `hooks/sqlite/nodb.go` (`//go:build nodb`)
- Create: `hooks/sqlite/nodb_test.go` (`//go:build nodb`)
- Create: `hooks/sqlite/available.go` (`//go:build !nodb`) with `func Available() bool { return true }`
- Create: `hooks/sqlite/available_nodb.go` (`//go:build nodb`) with `func Available() bool { return false }`

**Interfaces:**
- `func New(path string) (*Sink, error)` — empty path → no-op sink that still implements Hook (`OnUsage` returns immediately), `Path() string` empty.
- `type Sink struct` — `OnUsage`, `Close() error`, `Query(ctx, Query) ([]hooks.UsageEvent, error)`, `SetPath(path string) error` (close old, open new; empty = disable), `Path() string`
- `type Query struct { From, To time.Time; Provider, Model, Status, Q string; Limit int; Cursor string }`
- Schema exactly as spec. `modernc.org/sqlite`. File mode 0600.
- `OnUsage` never panics; lock timeout / busy → drop event (log nothing with secrets).
- `go get modernc.org/sqlite` — only this new direct dep besides existing.

- [ ] **Step 1: Failing test** (temp file, insert one event, Query all, assert RequestID). Second test: empty path New succeeds and Query returns nil.

- [ ] **Step 2:** `go test ./hooks/sqlite/ -count=1` FAIL.

- [ ] **Step 3: Implement.** WAL. Prepared insert. JSON-encode `DroppedFields` and `Media` as TEXT.

- [ ] **Step 4:** `go test ./hooks/sqlite/ -count=1` and `go test ./hooks/sqlite/ -tags nodb -count=1` (Available false, New returns no-op or error: **New with empty path always no-op; New with non-empty path under nodb returns error `sqlite disabled (build with default tags)`**).

- [ ] **Step 5: Commit** `feat: optional CGO-free sqlite usage sink`

---

### Task 5: Extract ChatGPT PKCE flow for dashboard

**Files:**
- Modify: `subauth/chatgpt.go`
- Test: `subauth/chatgpt_flow_test.go` (new) — fake token server + local redirect; or unit-test Start binds 1455/ephemeral and authorize URL contains `redirect_uri` + `state`

**Interfaces:**
- Produces:
  - `type ChatGPTFlow struct` with exported `AuthorizeURL string`
  - `func StartChatGPTFlow(ctx context.Context) (*ChatGPTFlow, error)` — listen `127.0.0.1:1455` else `:0`; serve `/auth/callback`; do **not** open a browser; do **not** wait
  - `func (f *ChatGPTFlow) Wait(ctx context.Context) (Credential, error)` — wait for code, exchange, return credential
  - `func (f *ChatGPTFlow) Close()`
  - `LoginChatGPT` rewritten as Start + OpenBrowser unless noBrowser + Wait + Close. Same observable CLI behavior (stderr messages can stay).

- [ ] **Step 1: Test** `StartChatGPTFlow` returns URL containing `auth.openai.com` and `code_challenge`; `Close` is safe twice. Existing `TestBuildChatGPTAuthorizeURL` and `TestListenLoopback` still pass.

- [ ] **Step 2:** `go test ./subauth/ -count=1` (may fail until refactor).

- [ ] **Step 3: Refactor.** PKCE verifier stays inside `ChatGPTFlow` unexported.

- [ ] **Step 4:** PASS. `go test ./cmd/gateway/ -count=1 -run Auth` still pass.

- [ ] **Step 5: Commit** `refactor: ChatGPT PKCE start/wait for dashboard`

---

### Task 6: Extract Grok device start + poll

**Files:**
- Modify: `subauth/grok.go`
- Test: `subauth/grok_device_test.go` with `httptest` for discovery + device + token pending then success.

**Interfaces:**
- `type GrokDevice struct { UserCode, VerificationURI, DeviceCode, TokenURL string; ExpiresIn, Interval int }`
- `func StartGrokDevice(ctx context.Context) (*GrokDevice, error)` — discovery + device-code POST. **No** auto-import. **No** browser open.
- `func (d *GrokDevice) Poll(ctx context.Context) (Credential, error)` — existing poll loop.
- `LoginGrok` keeps auto-import unless ForceDevice, then Start+print+Poll.

- [ ] **Step 1: httptest** covering Start returns UserCode; Poll maps `authorization_pending` then token.

- [ ] **Step 2–4:** TDD as usual. Existing grok import tests still pass.

- [ ] **Step 5: Commit** `refactor: Grok device start/poll for dashboard`

---

### Task 7: `dashboard` profiles HTTP (logout / disable)

**Files:**
- Create: `dashboard/dashboard.go` (New, Handler, types)
- Create: `dashboard/profiles.go`
- Create: `dashboard/dto.go` (ProfileDTO, `toDTO`, secret field names never tagged on DTO)
- Create: `dashboard/http.go` (JSON helpers, OpenAI-shaped errors)
- Create: `dashboard/profiles_test.go`

**Interfaces:**
- `type Options struct { Version string; AuthPath string; Ring *ring.Sink; SQLite sqliteSink; JSONLPath string; Dashboard config.Dashboard }`
- `sqliteSink` interface: `Query(...)`, `Path()`, `SetPath(string) error` so nodb can pass a stub.
- `func New(opts Options) *Server`
- `func (s *Server) Handler() http.Handler` — mux with the routes that exist in this task:
  - `GET /v1/dashboard/meta`
  - `GET /v1/dashboard/profiles`
  - `POST /v1/dashboard/profiles/{provider}/logout`
  - `POST /v1/dashboard/profiles/{provider}/disable`
- Meta: `{version, dashboard:true, sqlite: sqlite.Available(), nodb: !sqlite.Available(), ring_size, sqlite_path_set}`
- Profiles: `subauth.Load(authPath)`, `ValidProviders()`, `ListAccounts`. DTO per spec. **httptest body must not contain the access token string.**
- Logout: `store.Delete` / remove pool account; Save.
- Disable: `PutAccount` with Disabled flag.
- Unknown provider → 404.

- [ ] **Step 1: Test** TempDir credentials file with a chatgpt credential `access_token: "SECRETTOKEN"`; GET profiles 200; `bytes.Contains(body, []byte("SECRETTOKEN"))` must be false; usable true. Logout → missing. Disable pool account.

- [ ] **Step 2–4:** Implement. Reuse `subauth` only.

- [ ] **Step 5: Commit** `feat: dashboard profiles API`

---

### Task 8: `dashboard` usage + logs + SSE

**Files:**
- Create: `dashboard/usage.go`
- Create: `dashboard/logs.go`
- Create: `dashboard/jsonl_tail.go`
- Create: `dashboard/usage_test.go`
- Create: `dashboard/logs_test.go`

**Interfaces:**
- `GET /v1/dashboard/usage` and `GET /v1/dashboard/logs` and `GET /v1/dashboard/logs/stream` registered on the same mux.
- Source priority: sqlite if Path() non-empty; else jsonl file tail if `JSONLPath` is a real path (not stdout/stderr/empty); else ring.
- Totals computed in Go from the event slice (or SQL GROUP BY if sqlite).
- Logs: newest first, `limit` default 100 max 500, `next_cursor` = last request_id of page (simple).
- SSE: `Content-Type: text/event-stream`, `event: usage`, heartbeat comment every 15s (test can use a short hook or just the first event by writing to ring after subscribe — use a 100ms heartbeat in tests via unexported `heartbeat` var).
- Filters: provider, model, status, q substring on request_id.

- [ ] **Step 1: Tests** seed ring with 3 events; GET usage totals; GET logs order; GET logs `q`; stream receives one event (context cancel). JSONL file: write two lines, Options.JSONLPath set, Ring nil, sqlite path empty → usage from file.

- [ ] **Step 2–4.** Tail last 2MiB. Skip invalid JSON lines.

- [ ] **Step 5: Commit** `feat: dashboard usage and logs API`

---

### Task 9: `dashboard` OAuth start / status / complete / import

**Files:**
- Create: `dashboard/oauth.go`
- Create: `dashboard/oauth_test.go`

**Interfaces:**
- In-memory map `provider → pending` with mu, TTL 10m.
- `POST /v1/dashboard/oauth/start` `{provider}`
  - chatgpt: `StartChatGPTFlow`, store flow, goroutine `Wait` → save cred. Return `{kind:"redirect", authorize_url, state:"pending"}`.
  - grok: `StartGrokDevice`, goroutine `Poll` → save. Return `{kind:"device", user_code, verification_uri, expires_in, interval}`.
  - claude/gemini: 400 (wrong method).
- `GET /v1/dashboard/oauth/status?provider=`
- `POST /v1/dashboard/oauth/complete` claude `{provider, token}` → Put setup_token cred.
- `POST /v1/dashboard/oauth/import` `{provider}` → existing Import*FromCLI. Tests use env paths (`CODEX_HOME`, `GEMINI_AUTH_FILE`, etc.) pointing at temp files.

ChatGPT/Grok start tests **must not** call real IdP: inject function vars (`var startChatGPT = subauth.StartChatGPTFlow`) overridden in tests to return a fake URL without listening, and complete via status after fake save.

- [ ] **Step 1: Tests** with injected start funcs. Claude complete. Import chatgpt from temp `auth.json`. Body never contains tokens.

- [ ] **Step 2–4.**

- [ ] **Step 5: Commit** `feat: dashboard OAuth start/complete/import`

---

### Task 10: settings + CORS

**Files:**
- Create: `dashboard/settings.go`
- Modify: `dashboard/http.go` (CORS wrapper)
- Test: `dashboard/settings_test.go`

**Interfaces:**
- `GET /v1/dashboard/settings` `{ring_size, sqlite_path, sqlite_enabled: sqlite.Available() && Path()!="", jsonl_output}`
- `PUT /v1/dashboard/settings` `{sqlite_path?: string, ring_size?: int}` — Resize ring; SetPath sqlite. nodb + non-empty path → 400 code `sqlite_disabled`.
- If `Dashboard.CORSOrigin` non-empty, set `Access-Control-Allow-Origin` to that exact value, `Allow-Headers: Authorization, Content-Type, x-api-key`, `Allow-Methods: GET, POST, PUT, OPTIONS`. OPTIONS → 204. Never `*`.

- [ ] **Step 1–5.** Commit `feat: dashboard settings and CORS`

---

### Task 11: `cmd/gateway` mount + embed stub

**Files:**
- Create: `cmd/gateway/dist/index.html` (stub: `<!doctype html><title>Inja dashboard</title><p>Dashboard not built — npm run build in web/</p>`)
- Create: `cmd/gateway/ui.go` (`//go:build !noweb`) `//go:embed all:dist` `var uiFS embed.FS`
- Create: `cmd/gateway/ui_noweb.go` (`//go:build noweb`) `var uiFS embed.FS` empty
- Create: `cmd/gateway/operator.go` — wrap handler
- Modify: `cmd/gateway/main.go` — after `newGateway`, wrap with operator; log `ui on /ui` when enabled && !noweb
- Modify: `cmd/gateway/main_test.go` — existing tests still pass (serve stub sees wrapped handler)
- Create: `cmd/gateway/operator_test.go` — httptest: GET `/ui/` 200 contains `Dashboard`; GET `/v1/dashboard/meta` 200; with noweb build tag file not required in default tests
- Modify: `gateway.go` — **no API change required** if binary uses `WithHook`. Wire ring+sqlite in `operator.go` via `newGateway` replacement:

Change `newGateway` from `func(*config.Config) (http.Handler, error)` wrapping:

```go
func buildHandler(cfg *config.Config) (http.Handler, error) {
    ringSink := ring.New(cfg.Dashboard.Ring())
    var extra []gateway.Option
    extra = append(extra, gateway.WithHook(ringSink))
    sqlSink, err := sqlite.New(sqlitePath(cfg))
    // ...
    extra = append(extra, gateway.WithHook(sqlSink))
    h, err := gateway.New(cfg, extra...)
    return mountOperator(cfg, h, ringSink, sqlSink), nil
}
```

Keep `var newGateway = buildHandler` so tests still override.

`noweb` const: `const uiEnabled = true` in ui.go and `false` in ui_noweb.go.

SPA fallback: for `/ui/` prefix, if file exists in dist serve it; else `index.html`. Use `http.FileServer` on `http.FS` sub FS `dist` + a small wrapper.

- [ ] **Step 1:** Test `run` with config dashboard enabled, override serve, hit not needed if operator_test uses `mountOperator` directly.

- [ ] **Step 2–4:** `go test ./cmd/gateway/ -count=1` and `go test ./cmd/gateway/ -tags noweb -count=1`.

- [ ] **Step 5: Commit** `feat: serve /ui and /v1/dashboard from gateway binary`

---

### Task 12: React SPA in `web/`

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/tsconfig.app.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/api.ts`, `web/src/styles.css`
- Create: `web/src/pages/Profiles.tsx`, `Usage.tsx`, `Logs.tsx`, `Settings.tsx`, `Gate.tsx`
- Create: `web/src/pages/OAuthStatus.tsx` (poll status after start)
- Modify: `.gitignore` — `web/node_modules/`, `web/dist/`
- Do **not** commit `web/node_modules`. After build, copy to `cmd/gateway/dist/` is a CI/release step, not this commit (keep stub index.html). Add `web/README.md` one paragraph: `npm ci && npm run build` copies via `cp -R dist/. ../cmd/gateway/dist/`

**Interfaces (SPA):**
- `vite.config.ts`: `base: '/ui/'`, plugin react, `build.outDir: 'dist'`
- `VITE_GATEWAY_ORIGIN` default `''` (same origin)
- `api.ts`: `authHeader()` from `sessionStorage['inja.edge_key']`; `api(path)` fetch; on 401 throw `AuthError`
- Router basename `/ui`
- Profiles table + Login/Import/Disable/Logout
- Usage KPIs + inline SVG bars from `by_provider`
- Logs table + startStream() using fetch ReadableStream parsing `data: ` lines; fallback poll 2s
- Settings form sqlite path PUT
- No `EventSource`

**Visual:** dark, dense, operator-tool. Font: `"IBM Plex Sans", ui-sans-serif`. Accent: `#c8f542` on `#12140f`. No purple gradients, no Inter, no card grids with rounded-2xl.

- [ ] **Step 1:** Scaffold so `npx tsc --noEmit` / `npm run build` succeeds. No network in app code except `fetch` to gateway.

- [ ] **Step 2:** Implement pages. Keep each page <200 lines.

- [ ] **Step 3:** `npm run build` locally if node present; if not, still commit source. CI task 13 runs the build.

- [ ] **Step 4: Commit** `feat: operator dashboard SPA`

---

### Task 13: CI, Docker, release zip

**Files:**
- Modify: `.github/workflows/ci.yml` — smoke-binary: `curl -fsS http://127.0.0.1:18787/ui/` greps `html` or `Dashboard`. Add job `noweb` `go build -tags noweb` and `nodb` `go test ./hooks/sqlite/ -tags nodb`.
- Modify: `.github/workflows/release.yml` — setup-node 22, `npm ci && npm run build` in `web/`, `rm -rf cmd/gateway/dist && mkdir -p cmd/gateway/dist && cp -R web/dist/. cmd/gateway/dist/`, then go matrix; zip `inja-gateway-ui_${version}.zip` from `web/dist`.
- Modify: `Dockerfile` — extra stage `FROM node:22-alpine AS web` … copy dist into golang build context before `go build`.

- [ ] **Step 1–5.** Commit `ci: build dashboard SPA into binary and release zip`

---

### Task 14: Docs

**Files:**
- Modify: `README.md` — dashboard section (open `/ui`, edge_auth, tags, sqlite, ChatGPT uses localhost:1455)
- Modify: `CHANGELOG.md` `[Unreleased]`
- Modify: `CONTRIBUTING.md` database bullet
- Modify: `gateway.example.yaml` comments
- Modify: `AGENTS.md` one line
- Create or modify website doc: `website/src/content/docs/dashboard.mdx` (or a guides page following existing MDX layout — read one existing guide first)

- [ ] **Step 1–5.** Commit `docs: operator dashboard`

---

## Self-review

| Spec requirement | Task |
|---|---|
| Three artifacts (embed / noweb / spa zip) | 11, 13 |
| nodb tag | 4, 13 |
| Profiles + pool + no secrets | 7 |
| Usage ring + jsonl + sqlite | 3, 4, 8 |
| Logs = usage events + SSE fetch | 8, 12 |
| OAuth chatgpt localhost:1455, grok device, claude paste, gemini import | 5, 6, 9 |
| Settings sqlite path | 10 |
| edge_auth on API, /ui open | 2, 11 |
| library New unchanged role | 11 (WithHook only) |
| Docs | 14 |
