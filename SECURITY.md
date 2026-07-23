# Security

## Reporting

If you find a vulnerability in **Inja LLM Gateway** (`llm-gateway`), please open a **private** [security advisory](https://github.com/inja-online/llm-gateway/security/advisories/new) on the GitHub repository rather than filing a public issue.

## Scope notes

### Client authentication

- **Default:** the gateway does **not** authenticate clients. Deploy only on trusted networks, or put your own auth (API gateway, mTLS, service mesh) in front.
- **Optional edge auth** (`edge_auth.enabled`): static shared keys from `keys` and/or `keys_env` (comma-separated). Clients must send `Authorization: Bearer <key>` or `x-api-key: <key>`. Comparison is constant-time. **`GET /healthz` remains unauthenticated** for probes.
- Edge keys are **not** a full IAM product (no OAuth/OIDC, no per-tenant RBAC). Prefer external auth at the perimeter for multi-tenant SaaS.
- Edge auth is **distinct** from upstream credentials: set provider `api_key_env` so the server holds provider secrets while clients only present edge keys.
### Upstream credentials
- Client API keys are **forwarded** to upstream providers (or replaced via `api_key_env`), using each provider’s scheme (`Authorization`, `x-api-key`, or `x-goog-api-key` for native Gemini).
- **`auth: oauth2`:** the gateway POSTs to `oauth.token_url` (client_credentials or refresh_token). Prefer `client_id_env` / `client_secret_env` / `refresh_token_env`. Access/refresh tokens and client secrets are **never** logged; usage events only store `key_hash` of the access token.
- **`auth: client_bearer`:** always forward the client’s Bearer access token; never replace with `api_key_env`. Pair with edge auth for multi-tenant gateways that do not hold user OAuth at rest.
- **Vertex / ADC / SA** (`auth: adc` or `service_account`): Bearer from a `TokenSource`. Binary mode auto-builds a Google SA JWT exchange from `service_account_file` (or `GOOGLE_APPLICATION_CREDENTIALS`). Library mode may still inject via `SetTokenSource`. Mount SA JSON **read-only**; do not log its contents. No Google SDK is bundled.
- **Threat model:** a process with refresh tokens or SA keys is high privilege — treat the gateway host like a secret store. Prefer short-lived access tokens and workload identity where vendors support them.
- **Vendor ToS:** do not productize consumer subscription OAuth (ChatGPT/Claude consumer) as a multi-tenant resale API without checking provider terms.
- Treat logs carefully: usage events include a short `key_hash`, not the raw key — but request bodies may still contain secrets if clients send them.
### Ops hygiene
- The gateway **does not authenticate clients** by design. Deploy it only on trusted networks, or put your own auth (API gateway, mTLS, mesh policy) in front.
- Client API keys are **forwarded** to upstream providers (or replaced via `api_key_env`), using each provider’s scheme (`Authorization`, `x-api-key`, or `x-goog-api-key` for native Gemini). Treat logs carefully: usage events include a short `key_hash`, not the raw key — but request bodies may still contain secrets if clients send them.
- **Files API** (`/v1/files*`): the gateway does **not** persist uploads. Bytes exist only in the in-flight request/response (subject to the global body size limit). File objects live on the **upstream** provider account.
- **Stored Responses** (`GET/DELETE /v1/responses/{id}`): proxied only; no gateway-side response store.
- Prefer `hooks.jsonl.output: stdout` and your platform log pipeline over world-readable files.
- Containers run as **non-root** (distroless). Keep the root filesystem read-only in Kubernetes when possible (see `deploy/k8s/gateway.yaml`).
- Request/response body limit: configurable **`max_body_bytes`** (default **32 MiB**; see [README limits](README.md#limits--timeouts)). Oversize requests → HTTP **413** with a dialect-shaped error. Do not log multipart audio/image bytes.
- **Multipart / media review:** [docs/security-multipart-review.md](docs/security-multipart-review.md) — size limits, filename handling (no local open), content-type notes, SSRF posture (URI pass-through, gateway does not fetch `image_url` / `file_data` URIs), `key_hash` only on usage events.

## Supported versions

Only the latest `master` and the latest tagged release are supported for security fixes.
