# OAuth & token sources

Provider-side non–API-key authentication for **inja-online/llm-gateway** ([#104](https://github.com/inja-online/llm-gateway/issues/104)).

Edge auth (`edge_auth`) is **independent**: it authenticates callers of the gateway. This document covers how the gateway authenticates **to upstream providers**.

## Auth modes

| `auth` | Upstream credential |
|--------|---------------------|
| `api_key` (default) | Client key, or `api_key_env` when set |
| `bearer` | Same as api_key but always `Authorization: Bearer` |
| `client_bearer` | **Client** Bearer only — never `api_key_env` |
| `oauth2` | Built-in OAuth2 token endpoint (`oauth:` block) |
| `adc` / `service_account` | Bearer TokenSource (inject or auto SA file) |

## `auth: oauth2`

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
      # refresh_token_env: OAUTH_REFRESH_TOKEN   # → refresh_token grant
      scopes: ["api"]
      # grant: client_credentials   # optional override
      # audience: "..."
      # extra: { resource: "..." }
```

**Grants**

- `client_credentials` — default when no refresh credential is configured.
- `refresh_token` — default when `refresh_token` or `refresh_token_env` is set.

**Runtime**

1. On first request (and after expiry), `POST` `application/x-www-form-urlencoded` to `token_url`.
2. Cache `access_token` until `expires_in` (minus 30s skew). Concurrent callers share one refresh.
3. Upstream requests send `Authorization: Bearer <access_token>` (all kinds).
4. Usage `key_hash` hashes the **access token**, not the edge key.

Secrets: prefer `*_env`. Inline `client_id` / `client_secret` / `refresh_token` are allowed for tests only. Errors from the token endpoint never echo response bodies that might contain secrets.

## `auth: client_bearer`

For multi-tenant gateways where each client presents their own upstream OAuth access token:

```yaml
edge_auth:
  enabled: true
  keys_env: GATEWAY_EDGE_KEYS

providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: client_bearer
```

Clients send: edge key (per your deployment) **and** the provider access token in `Authorization: Bearer …` as required by your front door design. When only one `Authorization` header is available, put the **upstream** token there and terminate edge auth at an external proxy, **or** use `x-api-key` for edge and `Authorization` for upstream (edge middleware must accept `x-api-key`).

With `client_bearer`, `api_key_env` is **ignored** so a server-held key cannot accidentally replace user tokens.

## Google SA / ADC

```yaml
providers:
  vertex:
    kind: google
    base_url: "https://REGION-aiplatform.googleapis.com/v1/projects/PROJECT/locations/REGION/publishers/google"
    auth: service_account
    service_account_file: /secrets/sa.json
```

- Reads standard GCP SA JSON (`client_email`, `private_key`, optional `token_uri`).
- Signs a JWT and exchanges it at the token URL (`urn:ietf:params:oauth:grant-type:jwt-bearer`).
- Default scope: `https://www.googleapis.com/auth/cloud-platform`.
- `auth: adc` with `GOOGLE_APPLICATION_CREDENTIALS` pointing at the same JSON also auto-wires.
- Library embedders may still call `proxy.Server.SetTokenSource(name, ts)` to override.

No Google Cloud SDK is bundled.

## Provider notes & ToS

| Provider | Suggested pattern |
|----------|-------------------|
| **OpenAI** | API keys; WIF short-lived Bearer; optional `oauth2` / `client_bearer` for access tokens |
| **xAI** | Bearer (`openai_compat`); OAuth when vendor documents a token URL |
| **Anthropic** | Console API keys (`x-api-key`); operator-held OAuth refresh as `oauth2` Bearer — **check ToS** for consumer OAuth |
| **Google** | API key or SA/ADC TokenSource as above |
| **OpenCode** | Point `baseURL` at this gateway; use edge key + server `api_key_env`/`oauth2`, or `client_bearer` if OpenCode holds provider tokens |

Consumer subscription OAuth (ChatGPT, Claude.ai consumer) is often **restricted** for multi-user products. This gateway supports operator-held credentials; it is not a ToS workaround.

## Security checklist

- [ ] Secrets only via env or file mounts (mode `0600`, read-only volume)
- [ ] Never log Authorization headers, refresh tokens, or SA private keys
- [ ] Rotate refresh tokens / SA keys; prefer short-lived access tokens
- [ ] Edge keys ≠ provider credentials
- [ ] Multi-replica: each replica refreshes independently (no shared token DB in v1)

## Library injection

```go
srv := proxy.NewServer(cfg, hook)
srv.SetTokenSource("vertex", myTokenSource) // overrides auto-wire
http.ListenAndServe(cfg.Listen, srv.Handler())
```

`gateway.New` auto-wires from YAML the same way as `proxy.NewServer`.

## 401 force-refresh (one retry)

When `auth` uses a TokenSource (`oauth2` / `adc` / `service_account`) and the upstream responds **401** before the gateway has written to the client:

1. Invalidate the token cache (`CachingTokenSource.Invalidate`)
2. Fetch a fresh access token
3. Retry the upstream request **once**

This covers expired access tokens on chat/media/JSON paths. **Mid-SSE 401** after response headers have already been flushed to the client is **not** retried (cannot safely restart a stream). Prefer token TTLs with skew so access tokens are refreshed before expiry.

Static / non-invalidating TokenSources (e.g. test `StaticTokenSource`) do not retry.

## Non-goals (v1)

- Device-code / browser PKCE login CLI (use vendor CLIs; load refresh into env)
- Storing refresh tokens in a database
- Gateway-hosted OpenCode `/provider/{id}/oauth/*` IdP surface (document edge + Bearer first)
