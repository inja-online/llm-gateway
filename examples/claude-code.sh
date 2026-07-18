#!/usr/bin/env bash
# Run Claude Code against the gateway.
#
# Anthropic upstream  -> byte-exact passthrough.
# OpenAI-compatible   -> translated both ways (see the token-display caveat in
#                        the README).
#
# Usage:
#   KEY=sk-ant-... ./claude-code.sh                       # default provider
#   KEY=sk-... MODEL=ds/deepseek-chat ./claude-code.sh    # explicit provider
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8787}"
: "${KEY:?set KEY to an api key valid for the provider you route to}"

export ANTHROPIC_BASE_URL="$GATEWAY"
export ANTHROPIC_API_KEY="$KEY"
[ -n "${MODEL:-}" ] && export ANTHROPIC_MODEL="$MODEL"

echo "ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL"
echo "watch usage: tail -f the jsonl sink configured in gateway.yaml"
echo
exec claude "$@"
