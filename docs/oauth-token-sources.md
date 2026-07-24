# OAuth & token sources

**What:** how the gateway authenticates **to upstream providers** without (or in addition to) long-lived API keys.

**Not this page:** optional **edge auth** (`edge_auth`) that authenticates *clients of the gateway* — that is independent and covered in the README / Security docs.

## Mental model

```
  Client ──(edge key?)──► Gateway ──(upstream credential)──► OpenAI / Anthropic / Google / …
```

| Layer | Config | Header / secret |
|-------|--------|-----------------|
| **Edge** (optional) | `edge_auth` | Client `Authorization` or `x-api-key` must match gateway keys |
| **Upstream** | `providers.*.auth` | What the gateway sends **to the provider** |

Never put provider refresh tokens in `edge_auth.keys`.

---

## Auth modes (upstream)

| `auth` | What it does | When to use |
|--------|--------------|-------------|
| `api_key` (default) | Client key **or** `api_key_env` replacement | Classic API keys |
| `bearer` | Always `Authorization: Bearer` with client/env key | Hosts that only accept Bearer |
| `client_bearer` | **Only** client Bearer; **never** `api_key_env` | Multi-tenant: each user brings OAuth access token |
| `oauth2` | Fetch access token from `oauth.token_url` | Client credentials or refresh_token grant |
| `adc` / `service_account` | Bearer from TokenSource (SA JWT, `token_file`, inject) | Vertex / GCP / WIF file tokens |

---

## Mode details

### 1. `api_key` (default)

**How it works**

1. Read client credential from `Authorization: Bearer`, `x-api-key`, or `x-goog-api-key`.  
2. If `api_key_env` is set and non-empty, **replace** with that env value.  
3. Apply kind scheme: OpenAI Bearer · Anthropic `x-api-key` · Google `x-goog-api-key`.

```yaml
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    api_key_env: OPENAI_API_KEY   # clients only need edge key if edge_auth is on
```

```bash
export OPENAI_API_KEY=sk-...
curl -sS "$GW/v1/chat/completions" \
  -H "Authorization: Bearer $GATEWAY_EDGE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
```

---

### 2. `oauth2`

**What:** gateway is an OAuth2 **client**. It POSTs to a token endpoint, caches the access token, and sends `Authorization: Bearer <access_token>` upstream.

**How it works**

1. At process start, config is validated (`token_url`, client id/secret or refresh).  
2. On first request (or after expiry), form-POST to `token_url`.  
3. Cache until `expires_in` − 30s skew; concurrent requests share one refresh.  
4. On upstream **401** (before writing to the client): invalidate cache, refresh once, retry.  
5. Mid-SSE 401 after headers are flushed is **not** retried.

**YAML**

```yaml
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: oauth2
    oauth:
      token_url: "https://auth.example.com/oauth/token"
      client_id_env: OAUTH_CLIENT_ID
      client_secret_env: OAUTH_CLIENT_SECRET
      # refresh_token_env: OAUTH_REFRESH_TOKEN  # auto-selects refresh_token grant
      scopes: ["api"]
      # grant: client_credentials   # optional force
      # audience: "https://api.openai.com"
      # extra:
      #   resource: "..."
```

**Grants**

| Grant | Selected when | Form fields |
|-------|---------------|-------------|
| `client_credentials` | No refresh credential configured | `client_id`, `client_secret`, optional `scope` |
| `refresh_token` | `refresh_token` or `refresh_token_env` set | `refresh_token`, optional client id/secret |

**Env**

```bash
export OAUTH_CLIENT_ID=...
export OAUTH_CLIENT_SECRET=...
# or
export OAUTH_REFRESH_TOKEN=...
```

**Secrets:** prefer `*_env`. Inline `client_id` / `client_secret` / `refresh_token` are for tests only. Token endpoint errors never log response bodies (may echo secrets).

---

### 3. `client_bearer` (multi-tenant user OAuth)

**What:** every client presents **their own** upstream access token. The gateway never substitutes a server key.

```yaml
edge_auth:
  enabled: true
  keys_env: GATEWAY_EDGE_KEYS   # who may call the gateway

providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: client_bearer
    # api_key_env is ignored
```

**Client design**

| Header | Recommended use |
|--------|-----------------|
| `x-api-key` | Edge shared secret |
| `Authorization: Bearer` | Upstream OAuth access token |

If you only have one header, terminate edge auth at an external proxy and forward the upstream Bearer to the gateway.

---

### 4. Google SA / ADC (`service_account` / `adc`)

**What:** `Authorization: Bearer` from a short-lived Google access token.

**Auto-wire (binary):**

| Config | Behavior |
|--------|----------|
| `service_account_file: /path/sa.json` | JWT assertion → token endpoint |
| `GOOGLE_APPLICATION_CREDENTIALS` | Same when `auth: adc` and no file in YAML |
| `token_file: /path/token` | Read plain access token (WIF sidecar) — see [wif-recipes.md](wif-recipes.md) |

```yaml
providers:
  vertex:
    kind: google
    base_url: "https://us-central1-aiplatform.googleapis.com/v1/projects/PROJECT/locations/us-central1/publishers/google"
    auth: service_account
    service_account_file: /secrets/vertex-sa.json
```

Mount SA JSON **read-only**, mode `0600`. No Google Cloud SDK is bundled.

**Library inject**

```go
srv := proxy.NewServer(cfg, hook)
srv.SetTokenSource("vertex", mySource) // overrides auto-wire
http.ListenAndServe(cfg.Listen, srv.Handler())
```

---

## End-to-end recipes

### A. Edge auth + server-held API key (most common)

```yaml
edge_auth: { enabled: true, keys_env: GATEWAY_EDGE_KEYS }
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    api_key_env: OPENAI_API_KEY
```

Clients send only the edge key; gateway holds `sk-…`.

### B. Edge auth + OAuth2 client credentials

```yaml
edge_auth: { enabled: true, keys_env: GATEWAY_EDGE_KEYS }
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: oauth2
    oauth:
      token_url: "https://…"
      client_id_env: OIDC_CLIENT_ID
      client_secret_env: OIDC_CLIENT_SECRET
```

### C. Multi-tenant user tokens

```yaml
edge_auth: { enabled: true, keys_env: GATEWAY_EDGE_KEYS }
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: client_bearer
```

### D. Vertex service account

See [vertex-dual-path.md](vertex-dual-path.md) + SA block above.

### E. OpenCode / Claude Code / Codex

Point the tool `baseURL` at the gateway (Claude Code: `ANTHROPIC_BASE_URL`). Prefer **edge key + server credentials**.

### F. Consumer subscription OAuth (ChatGPT / Claude / SuperGrok)

Interactive login (no long-lived API keys):

```bash
./llm-gateway auth login chatgpt   # Codex PKCE → ChatGPT subscription
./llm-gateway auth login claude    # setup-token / Claude Code OAuth
./llm-gateway auth login grok      # SuperGrok device-code OAuth
./llm-gateway auth status
```

Wire providers with the local credential store:

```yaml
providers:
  chatgpt:
    kind: openai_compat
    base_url: "https://chatgpt.com/backend-api/codex"
    auth: oauth2
    oauth:
      credentials: chatgpt   # chatgpt | claude | grok
```

Full guide: [claude-code-multi.md](claude-code-multi.md) · config [`examples/configs/claude-code-subscriptions.yaml`](https://github.com/inja-online/llm-gateway/blob/master/examples/configs/claude-code-subscriptions.yaml).

**ToS:** personal use of accounts you own only; do not productize multi-tenant resale of consumer OAuth. Re-read OpenAI / Anthropic / xAI terms.

#### Upstream headers the gateway injects

For `oauth.credentials` providers the gateway adds CLI-compatible headers after Bearer auth:

| Credentials | Injected (when missing / always for identity) |
|-------------|-----------------------------------------------|
| `claude` | `Authorization: Bearer` (never `x-api-key`); `anthropic-version: 2023-06-01`; `anthropic-beta` defaults including **`oauth-2025-04-20`** (merged with client betas); `X-App: cli` |
| `chatgpt` | Codex-like `User-Agent` / `Originator: codex-tui`; **`Chatgpt-Account-Id`** from JWT claims stored at login |
| `grok` | Plain Bearer only (no extra headers today) |

ChatGPT login/refresh parses the access/id JWT for `chatgpt_account_id` and stores it on the credential.

#### Model list (`GET /v1/models`)

When a provider uses `oauth.credentials`:

- Aliases and `provider/model` targets for that provider appear **only if** the local store has a usable credential (`llm-gateway auth login` / import).
- Logged-in providers also get a static subscription catalog (Claude / Codex / xAI ids) as `provider/<upstream-id>`.
- Non-subscription providers (API keys) are unchanged.
- `?live=1` still fans out to hosts that expose `/models` (Codex backend usually does not).

---

## Observability

- Usage events include `key_hash` = first 12 hex of SHA-256 of the **upstream** credential (access token or API key), never the raw secret.  
- Do not log `Authorization`, refresh tokens, or SA private keys.

## Security checklist

- [ ] Secrets via env or file mounts only  
- [ ] SA / token files read-only, `0600`  
- [ ] Edge keys ≠ provider credentials  
- [ ] Prefer short-lived access tokens  
- [ ] Review vendor ToS for consumer OAuth  

## Related

- [WIF recipes](wif-recipes.md)  
- [Vertex dual-path](vertex-dual-path.md)  
- [Realtime WebSocket auth](realtime-websocket.md)  
- [SECURITY.md](https://github.com/inja-online/llm-gateway/blob/master/SECURITY.md)  
