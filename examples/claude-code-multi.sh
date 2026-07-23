#!/usr/bin/env bash
# Run Claude Code through Inja LLM Gateway with any provider combination.
#
# Usage:
#   ./examples/claude-code-multi.sh                    # interactive picker
#   ./examples/claude-code-multi.sh multi              # Claude + GPT + Grok
#   ./examples/claude-code-multi.sh gpt                # GPT only
#   ./examples/claude-code-multi.sh grok               # Grok only (4.5 + composer-2.5)
#   ./examples/claude-code-multi.sh gpt+grok           # GPT + Grok, no Claude
#   ./examples/claude-code-multi.sh claude+gpt
#   ./examples/claude-code-multi.sh list
#
# Gateway (subscription OAuth recommended):
#   ./llm-gateway -config examples/configs/claude-code-subscriptions.yaml
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=shell/claude-code-helpers.sh
# Load helpers (profiles + background HTTPS gateway) so combos work in bash/zsh.
source "$ROOT/examples/shell/claude-code-helpers.sh"

PROFILE="${1:-${PROFILE:-${CC_PROVIDERS:-}}}"

if [[ $# -gt 0 ]]; then
  case "$1" in
    list|help|-h|--help)
      PROFILE="$1"
      shift
      ;;
    -*)
      # flags only → interactive / env profile
      ;;
    *)
      # treat as profile if it looks like one
      if [[ "$1" =~ ^(claude|gpt|grok|multi|all|openai|xai|chatgpt|anthropic)([+_,-](claude|gpt|grok|openai|xai|chatgpt|anthropic|multi|all))*$ ]]; then
        PROFILE="$1"
        shift
      elif [[ -n "${PROFILE}" ]]; then
        :
      else
        PROFILE=""
      fi
      ;;
  esac
fi

usage() {
  cat <<'EOF'
Usage: claude-code-multi.sh [profile|combo] [claude args...]

Any combination of: claude, gpt, grok
  multi | claude+gpt+grok
  gpt | grok | claude
  gpt+grok | claude+gpt | claude+grok
  gpt,grok   (comma ok)

Grok defaults:  grok-4.5 (heavy) · composer-2.5 (fast)
GPT defaults:   o3 / gpt / gpt-mini
Claude:         opus / sonnet / haiku

Env: GATEWAY KEY CC_MODEL CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_PROVIDERS
     CC_GROK_HEAVY CC_GROK_FAST CC_GPT_HEAVY CC_GPT_MID CC_GPT_FAST

Examples:
  KEY=local-dev ./examples/claude-code-multi.sh gpt
  KEY=local-dev ./examples/claude-code-multi.sh grok
  KEY=local-dev ./examples/claude-code-multi.sh gpt+grok
  CC_MODEL=composer-2.5 ./examples/claude-code-multi.sh grok
EOF
}

if [[ "${PROFILE}" == "help" || "${PROFILE}" == "-h" || "${PROFILE}" == "--help" ]]; then
  usage
  _inja_cc_list_profiles
  exit 0
fi
if [[ "${PROFILE}" == "list" ]]; then
  _inja_cc_list_profiles
  exit 0
fi

if [[ -z "${PROFILE}" ]]; then
  if [[ -t 0 ]]; then
    echo "Select providers for Claude Code (gateway: $GATEWAY)"
    echo "  1) multi       — Claude + GPT + Grok"
    echo "  2) gpt         — GPT only"
    echo "  3) grok        — Grok only (4.5 + composer-2.5)"
    echo "  4) gpt+grok    — GPT + Grok (no Claude)"
    echo "  5) claude      — Claude only"
    echo "  6) claude+gpt"
    echo "  7) claude+grok"
    read -r -p "Choice [1-7, default 1] or type combo (e.g. gpt+grok): " choice
    case "${choice:-1}" in
      1|"") PROFILE=multi ;;
      2) PROFILE=gpt ;;
      3) PROFILE=grok ;;
      4) PROFILE=gpt+grok ;;
      5) PROFILE=claude ;;
      6) PROFILE=claude+gpt ;;
      7) PROFILE=claude+grok ;;
      *) PROFILE="$choice" ;;
    esac
  else
    PROFILE=multi
  fi
fi

export KEY="${KEY:-${GATEWAY_EDGE_KEY:-${ANTHROPIC_API_KEY:-local-dev}}}"

# Start HTTPS gateway in background (unless already up), then apply profile + run claude.
if [[ "${CC_SKIP_GATEWAY_UP:-}" != "1" ]]; then
  cc-gateway-up || exit $?
fi

if ! _inja_cc_apply_combo "$PROFILE"; then
  usage >&2
  exit 2
fi

if ! command -v claude >/dev/null 2>&1; then
  echo "claude CLI not found on PATH. Install Claude Code: https://code.claude.com/docs" >&2
  exit 127
fi

echo "profile=$CC_GATEWAY_PROFILE  providers=$CC_GATEWAY_PROVIDERS  gateway=$ANTHROPIC_BASE_URL"
echo "  model=$ANTHROPIC_MODEL  opus=$ANTHROPIC_DEFAULT_OPUS_MODEL  sonnet=$ANTHROPIC_DEFAULT_SONNET_MODEL  haiku=$ANTHROPIC_DEFAULT_HAIKU_MODEL"
echo "  /model $CC_MODEL_HINTS"
echo

exec claude "$@"
