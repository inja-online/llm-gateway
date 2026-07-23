# Vertex AI dual-path

**What:** run native Gemini (`kind: google`) against **Vertex AI** or **Google AI Studio** by choosing `base_url` + auth.

**How:** client paths stay `/v1beta/models/{model}:generateContent` (and Live, platform proxies). Egress appends `/models/…` to `base_url`, so Vertex `base_url` must already include the project/location **publisher** prefix.

## AI Studio vs Vertex

| Path | Host pattern | Auth |
|------|--------------|------|
| **AI Studio** | `generativelanguage.googleapis.com/v1beta` | API key (`x-goog-api-key`) or OAuth |
| **Vertex** | `{LOCATION}-aiplatform.googleapis.com/v1/projects/{P}/locations/{L}/publishers/google` | ADC / service account / token_file |

## Config helper

```go
base := config.VertexBaseURL("my-project", "us-central1")
// https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google
```

YAML:

```yaml
providers:
  vertex:
    kind: google
    base_url: "https://us-central1-aiplatform.googleapis.com/v1/projects/PROJECT/locations/us-central1/publishers/google"
    auth: service_account
    service_account_file: /secrets/sa.json
```

Global endpoint: location `global` → host `aiplatform.googleapis.com`.

Gateway paths remain `/v1beta/models/{model}:generateContent` on the **client** side; the provider `base_url` already includes the Vertex publisher prefix so egress `Path()` appends `/models/…` correctly.

## IAM

Prefer Workload Identity + `token_file` or SA JSON (see [wif-recipes.md](wif-recipes.md)). Do not embed long-lived user OAuth for multi-tenant products.
