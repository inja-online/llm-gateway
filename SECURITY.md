# Security

## Reporting

If you find a vulnerability in **llm-gateway**, please open a **private** security advisory on the GitHub repository (or contact the maintainers) rather than filing a public issue.

## Scope notes

- The gateway **does not authenticate clients** by design. Deploy it only on trusted networks, or put your own auth (API gateway, mTLS, mesh policy) in front.
- Client API keys are **forwarded** to upstream providers (or replaced via `api_key_env`). Treat logs carefully: usage events include a short `key_hash`, not the raw key — but request bodies may still contain secrets if clients send them.
- Prefer `hooks.jsonl.output: stdout` and your platform log pipeline over world-readable files.
- Containers run as **non-root** (distroless). Keep the root filesystem read-only in Kubernetes when possible (see `deploy/k8s/gateway.yaml`).

## Supported versions

Only the latest `master` and the latest tagged release are supported for security fixes.
