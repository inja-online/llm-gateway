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
- **Vertex / ADC** (`auth: adc` or `service_account`): the gateway uses an injected `TokenSource` (`Authorization: Bearer`). Real Google ADC is optional and not bundled as a heavy SDK dependency — inject tokens from your runtime or a refresh sidecar. If you mount a service-account JSON file, mount it **read-only** and restrict filesystem permissions; do not log its contents.
- Treat logs carefully: usage events include a short `key_hash`, not the raw key — but request bodies may still contain secrets if clients send them.
### Ops hygiene
- The gateway **does not authenticate clients** by design. Deploy it only on trusted networks, or put your own auth (API gateway, mTLS, mesh policy) in front.
- Client API keys are **forwarded** to upstream providers (or replaced via `api_key_env`), using each provider’s scheme (`Authorization`, `x-api-key`, or `x-goog-api-key` for native Gemini). Treat logs carefully: usage events include a short `key_hash`, not the raw key — but request bodies may still contain secrets if clients send them.
- **Files API** (`/v1/files*`): the gateway does **not** persist uploads. Bytes exist only in the in-flight request/response (subject to the global body size limit). File objects live on the **upstream** provider account.
- **Stored Responses** (`GET/DELETE /v1/responses/{id}`): proxied only; no gateway-side response store.
- Prefer `hooks.jsonl.output: stdout` and your platform log pipeline over world-readable files.
- Containers run as **non-root** (distroless). Keep the root filesystem read-only in Kubernetes when possible (see `deploy/k8s/gateway.yaml`).
- Request/response body limit: **32 MiB** (see README limits). Do not log multipart audio/image bytes.

## Supported versions

Only the latest `master` and the latest tagged release are supported for security fixes.
