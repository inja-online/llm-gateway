# Workload identity federation recipes

Short-lived upstream tokens for **inja-online/llm-gateway** without long-lived `sk-…` keys in the process environment. Extends [OAuth & token sources](oauth-token-sources.md) and [#104](https://github.com/inja-online/llm-gateway/issues/104) / [#164](https://github.com/inja-online/llm-gateway/issues/164).

The gateway stays **stateless**: it does not run a cloud STS. Exchange happens **outside** (sidecar, init container, CI step, or library `SetTokenSource`), then the gateway consumes either:

| Mechanism | Config |
|-----------|--------|
| OAuth2 client credentials / refresh | `auth: oauth2` + `oauth:` block |
| Google SA JSON JWT | `auth: service_account` + `service_account_file` |
| Plain access token file (WIF sidecar) | `auth: adc` + `token_file` |
| Injected source (library) | `proxy.Server.SetTokenSource` |

---

## Pattern A — Token file (universal)

A sidecar (or CSI / projected volume refresh) writes a bearer access token to a path the gateway can read:

```yaml
providers:
  openai:
    kind: openai
    base_url: "https://api.openai.com/v1"
    auth: adc
    token_file: /var/run/secrets/openai-access-token
```

- File contents: raw access token only (no `Bearer ` prefix); whitespace trimmed.
- Cached ~2 minutes then re-read (rotation-friendly).
- Mount **read-only**; mode `0600`; never log file contents.

Works for OpenAI WIF, cloud OIDC exchanges, vault agents, etc. as long as something refreshes the file.

---

## Pattern B — OpenAI workload identity federation

1. Configure OpenAI project **workload identity** (OIDC → OpenAI access token). See [OpenAI WIF guide](https://developers.openai.com/api/docs/guides/workload-identity-federation).
2. Exchange the cloud OIDC token for an OpenAI access token in a sidecar or CI step.
3. Write the access token to `token_file` **or** export it and use:

```yaml
# If you already hold a refreshable OAuth client for the token endpoint:
auth: oauth2
oauth:
  token_url: "https://..."   # your exchange / token URL
  client_id_env: ...
  client_secret_env: ...
  # or refresh_token_env after initial exchange
```

**CI (GitHub Actions sketch):**

```yaml
# 1) Request OIDC JWT from Actions
# 2) Exchange with OpenAI / your broker for an access token
# 3) Write token to a file or set env for api_key_env for the job duration only
- run: |
    echo "$OPENAI_ACCESS_TOKEN" > /tmp/oai.token
    chmod 600 /tmp/oai.token
```

Prefer job-scoped tokens; do not commit tokens.

---

## Pattern C — Google Cloud (Vertex)

**Service account JSON (simplest binary mode):**

```yaml
providers:
  vertex:
    kind: google
    base_url: "https://us-central1-aiplatform.googleapis.com/v1/projects/PROJECT/locations/us-central1/publishers/google"
    auth: service_account
    service_account_file: /secrets/vertex-sa.json
```

**GKE Workload Identity (no JSON key):**

1. Bind KSA → GSA with `roles/aiplatform.user` (or tighter).
2. Sidecar or `gcloud auth print-access-token` equivalent writes token to `token_file`, **or** inject `SetTokenSource` from the metadata server in library mode.
3. Config:

```yaml
auth: adc
token_file: /var/run/secrets/gcp-access-token
```

`GOOGLE_APPLICATION_CREDENTIALS` pointing at SA JSON also auto-wires for `auth: adc`.

---

## Pattern D — AWS / Azure → provider token

The gateway does not speak STS/IMDS natively.

1. Use cloud IRSA / Managed Identity to obtain a cloud credential.
2. Exchange via your broker (or vendor WIF) for the **provider** access token.
3. Feed the gateway via `token_file` or `auth: oauth2` against the broker’s token URL.

Keep the exchange **out of** the hot request path when possible (sidecar refresh loop).

---

## Pattern E — GitHub Actions OIDC

```text
GHA OIDC JWT → vendor / broker token endpoint → access token → token_file or env
```

- Restrict `aud` / `sub` claims on the trust policy.
- Short TTL tokens; never upload them as workflow artifacts.
- Gateway job example: start `llm-gateway` with `token_file` after the exchange step.

---

## Multi-tenant vs operator-held

| Mode | Use when |
|------|----------|
| `auth: oauth2` / SA / `token_file` | **Operator-held** credential; clients use edge auth only |
| `auth: client_bearer` | Each client presents their own upstream OAuth access token |
| `api_key_env` | Classic static key (still fine for many deployments) |

Do not put provider refresh tokens in `edge_auth.keys`.

---

## Threat model (short)

- Process with SA keys or refresh tokens is **high privilege** — treat the host like a secret store.
- `token_file` is world-readable if mis-mounted; use secret volumes and pod security.
- TLS to upstream is independent (see Realtime TLS notes in SECURITY.md).
- ToS: consumer subscription OAuth is not a substitute for WIF in multi-user products.

---

## Library injection

```go
srv := proxy.NewServer(cfg, hook)
srv.SetTokenSource("openai", proxy.FileTokenSource{Path: "/run/token"})
// or CachingTokenSource / custom OIDC exchange TokenSource
```

---

## Checklist

- [ ] Prefer short-lived access tokens over static API keys
- [ ] Secrets via env / file mounts only
- [ ] Sidecar refresh before token expiry (skew)
- [ ] Edge auth orthogonal to upstream WIF
- [ ] No secrets in logs or usage events (only `key_hash`)
