# Media & chat fixtures

Naming: `{dialect}_{modality}_{case}.json`

Rules:

- No PII; tiny base64 blobs only (`YQ==`).
- Produced offline; CI never hits public provider APIs.
- Golden files lock Gateway Media Contract v1 wire shapes.

Examples:

- `anthropic_image_gen_request.json` — POST `/v1/images` body
- `google_image_predict_response.json` — Imagen `:predict` predictions
- `openai_image_generations_response.json` — Images API data array
