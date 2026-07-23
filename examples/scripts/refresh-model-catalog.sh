#!/usr/bin/env bash
# Refresh / print live model catalogs from a running gateway (or dry docs).
#
# Usage:
#   # Gateway already up (HTTPS local):
#   ./examples/scripts/refresh-model-catalog.sh
#
#   GATEWAY=https://127.0.0.1:8787 KEY=local-dev ./examples/scripts/refresh-model-catalog.sh
#
# Prints:
#   1) config aliases (GET /v1/models)
#   2) live merged catalog (GET /v1/models?live=1) when credentials work
#
# Agents: when updating examples/configs/* aliases, run this (or curl live=1)
# against a gateway with real subscription tokens and pin short aliases to
# currently advertised full ids. See AGENTS.md "Model aliases".
set -euo pipefail

GATEWAY="${GATEWAY:-https://127.0.0.1:8787}"
KEY="${KEY:-${GATEWAY_EDGE_KEY:-local-dev}}"
CURL=(curl -skS --max-time 20)
AUTH=(-H "Authorization: Bearer $KEY" -H "Content-Type: application/json")

echo "== config catalog (aliases only) $GATEWAY/v1/models =="
"${CURL[@]}" "${AUTH[@]}" "$GATEWAY/v1/models" | {
  if command -v jq >/dev/null 2>&1; then
    jq -r '.data[]?.id' | sort
  else
    cat
  fi
}

echo
echo "== live catalog $GATEWAY/v1/models?live=1 =="
echo "(merges upstream GET /models for providers that have credentials)"
"${CURL[@]}" "${AUTH[@]}" "$GATEWAY/v1/models?live=1" | {
  if command -v jq >/dev/null 2>&1; then
    jq -r '.data[]?.id' | sort
  else
    cat
  fi
}

echo
echo "Suggested short aliases (edit YAML targets to match live ids above):"
cat <<'EOF'
  # Claude  — prefer current ids from platform.claude.com/docs
  sonnet: anthropic/<live-sonnet-id>
  opus:   anthropic/<live-opus-id>
  haiku:  anthropic/<live-haiku-id>

  # ChatGPT / Codex
  gpt:  chatgpt/<live-balanced-id>
  sol:  chatgpt/<live-flagship-id>
  luna: chatgpt/<live-fast-id>

  # xAI
  grok:     xai/<live-flagship-id>
  composer: xai/<live-build-or-coding-id>
EOF
