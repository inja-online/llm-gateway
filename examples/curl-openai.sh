#!/usr/bin/env bash
# Non-streaming and streaming requests through the gateway.
# Usage: GATEWAY=http://localhost:8787 KEY=sk-... MODEL=deepseek/deepseek-chat ./curl-openai.sh
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8787}"
MODEL="${MODEL:-deepseek/deepseek-chat}"
: "${KEY:?set KEY to an api key valid for the target provider}"

echo "== non-streaming =="
curl -sS "$GATEWAY/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Say hi in one word.\"}]}"
echo

echo "== streaming =="
curl -sSN "$GATEWAY/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$MODEL\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"Count to 5.\"}]}"
echo
