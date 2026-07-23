#!/usr/bin/env bash
# Run Claude Code against the gateway (minimal launcher).
#
# Anthropic upstream  -> byte-exact passthrough.
# OpenAI-compatible   -> translated both ways (see the token-display caveat in
#                        the README).
#
# Usage:
#   KEY=sk-ant-... ./claude-code.sh                       # default provider
#   KEY=sk-... MODEL=ds/deepseek-chat ./claude-code.sh    # explicit provider
#
# Multi-provider (Claude + GPT + Grok) with profiles:
#   ./examples/claude-code-multi.sh multi
#   source examples/shell/claude-code-helpers.sh && cc-multi
# Docs: docs/claude-code-multi.md
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8787}"
: "${KEY:?set KEY to an api key valid for the provider you route to}"

export ANTHROPIC_BASE_URL="$GATEWAY"
export ANTHROPIC_API_KEY="$KEY"
export ANTHROPIC_AUTH_TOKEN="${ANTHROPIC_AUTH_TOKEN:-$KEY}"
[ -n "${MODEL:-}" ] && export ANTHROPIC_MODEL="$MODEL"

echo "ANTHROPIC_BASE_URL=$ANTHROPIC_BASE_URL"
echo "watch usage: tail -f the jsonl sink configured in gateway.yaml"
echo
exec claude "$@"
