# shellcheck shell=bash
# Shared Claude Code profile resolver for any provider combination.
#
# Profiles (names or combos):
#   claude | gpt | grok
#   multi                 = claude+gpt+grok
#   gpt+grok | gpt-grok | gpt,grok
#   claude+gpt | claude+grok | claude+gpt+grok
#   any permutation with + , or - separators
#
# Slot mapping (Claude Code opus / sonnet / haiku):
#   Providers present decide which models fill each slot.
#   Grok defaults: opus/main heavy → grok-4.5, fast → composer-2.5
#   GPT defaults:  heavy → o3 / gpt, fast → gpt-mini
#   Claude defaults: opus / sonnet / haiku aliases
#
# Env overrides (always win):
#   CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_MODEL CC_SMALL_FAST_MODEL
#   CC_PROVIDERS=gpt,grok   # same as profile gpt+grok

# Normalize a profile string → sorted unique provider list (space-separated).
# Prints: "claude" | "gpt" | "grok" | "claude gpt" | "gpt grok" | "claude gpt grok" | ...
_inja_cc_normalize_providers() {
  local raw="${1:-}"
  raw="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]' | tr ',/|' '+++' | tr -s '+- ' '+')"
  # named shortcuts
  case "$raw" in
    ""|multi|all|full|cgg|cgx) raw="claude+gpt+grok" ;;
    openai|chatgpt) raw="gpt" ;;
    xai|supergrok) raw="grok" ;;
    anthropic) raw="claude" ;;
    gpt+xai|xai+gpt|openai+grok|grok+openai) raw="gpt+grok" ;;
    claude+openai|openai+claude) raw="claude+gpt" ;;
    claude+xai|xai+claude) raw="claude+grok" ;;
  esac

  local -a want=()
  local part
  IFS='+' read -r -a parts <<<"$raw"
  for part in "${parts[@]}"; do
    part="${part// /}"
    case "$part" in
      claude|anthropic) want+=("claude") ;;
      gpt|openai|chatgpt) want+=("gpt") ;;
      grok|xai|supergrok) want+=("grok") ;;
      multi|all) want+=("claude" "gpt" "grok") ;;
      "") ;;
      *)
        echo "unknown provider in profile: $part (use claude|gpt|grok)" >&2
        return 2
        ;;
    esac
  done

  # unique preserve order claude,gpt,grok
  local out="" has_c=0 has_g=0 has_x=0
  local p
  for p in "${want[@]}"; do
    case "$p" in
      claude) has_c=1 ;;
      gpt) has_g=1 ;;
      grok) has_x=1 ;;
    esac
  done
  [[ $has_c -eq 1 ]] && out="${out}claude "
  [[ $has_g -eq 1 ]] && out="${out}gpt "
  [[ $has_x -eq 1 ]] && out="${out}grok "
  out="${out% }"
  if [[ -z "$out" ]]; then
    echo "empty provider set" >&2
    return 2
  fi
  printf '%s' "$out"
}

# Given providers string "claude gpt grok", set OPUS_M SONNET_M HAIKU_M MAIN_M PROFILE_LABEL
_inja_cc_map_slots() {
  local providers="$1"
  local has_c=0 has_g=0 has_x=0
  # Word-boundary-ish: space-padded match
  local pad=" $providers "
  [[ "$pad" == *" claude "* ]] && has_c=1
  [[ "$pad" == *" gpt "* ]] && has_g=1
  [[ "$pad" == *" grok "* ]] && has_x=1

  local n=$((has_c + has_g + has_x))
  PROFILE_LABEL="${providers// /+}"

  # Defaults per provider family (gateway aliases)
  # Grok: heavy = grok-4.5, fast/coding = composer-2.5
  local g_heavy="${CC_GROK_HEAVY:-grok-4.5}"
  local g_fast="${CC_GROK_FAST:-composer-2.5}"
  local gpt_heavy="${CC_GPT_HEAVY:-o3}"
  local gpt_mid="${CC_GPT_MID:-gpt}"
  local gpt_fast="${CC_GPT_FAST:-gpt-mini}"
  local c_opus="${CC_CLAUDE_OPUS:-opus}"
  local c_sonnet="${CC_CLAUDE_SONNET:-sonnet}"
  local c_haiku="${CC_CLAUDE_HAIKU:-haiku}"

  if [[ $n -eq 1 ]]; then
    if [[ $has_c -eq 1 ]]; then
      OPUS_M="$c_opus"; SONNET_M="$c_sonnet"; HAIKU_M="$c_haiku"; MAIN_M="$c_sonnet"
    elif [[ $has_g -eq 1 ]]; then
      OPUS_M="$gpt_heavy"; SONNET_M="$gpt_mid"; HAIKU_M="$gpt_fast"; MAIN_M="$gpt_mid"
    else
      # grok only — heavy + composer
      OPUS_M="$g_heavy"; SONNET_M="$g_heavy"; HAIKU_M="$g_fast"; MAIN_M="$g_heavy"
    fi
  elif [[ $n -eq 2 ]]; then
    if [[ $has_c -eq 1 && $has_g -eq 1 ]]; then
      # claude + gpt
      OPUS_M="$c_opus"; SONNET_M="$gpt_mid"; HAIKU_M="$gpt_fast"; MAIN_M="$c_sonnet"
    elif [[ $has_c -eq 1 && $has_x -eq 1 ]]; then
      # claude + grok
      OPUS_M="$c_opus"; SONNET_M="$g_heavy"; HAIKU_M="$g_fast"; MAIN_M="$c_sonnet"
    else
      # gpt + grok (no Claude)
      OPUS_M="$g_heavy"; SONNET_M="$gpt_mid"; HAIKU_M="$g_fast"; MAIN_M="$gpt_mid"
    fi
  else
    # all three
    OPUS_M="$c_opus"; SONNET_M="$gpt_mid"; HAIKU_M="$g_fast"; MAIN_M="$c_sonnet"
  fi

  # Explicit env overrides always win
  OPUS_M="${CC_OPUS_MODEL:-$OPUS_M}"
  SONNET_M="${CC_SONNET_MODEL:-$SONNET_M}"
  HAIKU_M="${CC_HAIKU_MODEL:-$HAIKU_M}"
  MAIN_M="${CC_MODEL:-$MAIN_M}"
}

# Apply providers to Claude Code env. Args: profile-or-combo
_inja_cc_apply_combo() {
  local raw="${1:-${CC_PROVIDERS:-multi}}"
  local providers
  providers="$(_inja_cc_normalize_providers "$raw")" || return $?

  local OPUS_M SONNET_M HAIKU_M MAIN_M PROFILE_LABEL
  _inja_cc_map_slots "$providers"

  local gateway="${GATEWAY:-http://localhost:8787}"
  local key="${KEY:-${GATEWAY_EDGE_KEY:-${ANTHROPIC_API_KEY:-gateway}}}"

  export ANTHROPIC_BASE_URL="$gateway"
  export ANTHROPIC_API_KEY="$key"
  export ANTHROPIC_AUTH_TOKEN="${ANTHROPIC_AUTH_TOKEN:-$key}"
  export ANTHROPIC_MODEL="$MAIN_M"
  export ANTHROPIC_DEFAULT_OPUS_MODEL="$OPUS_M"
  export ANTHROPIC_DEFAULT_SONNET_MODEL="$SONNET_M"
  export ANTHROPIC_DEFAULT_HAIKU_MODEL="$HAIKU_M"
  export ANTHROPIC_SMALL_FAST_MODEL="${CC_SMALL_FAST_MODEL:-$HAIKU_M}"
  export CC_GATEWAY_PROFILE="$PROFILE_LABEL"
  export CC_GATEWAY_PROVIDERS="$providers"

  # Hint list for /model (printed by callers)
  CC_MODEL_HINTS=""
  case "$providers" in
    *claude*) CC_MODEL_HINTS+="sonnet opus haiku claude " ;;
  esac
  case "$providers" in
    *gpt*) CC_MODEL_HINTS+="gpt gpt-mini o3 " ;;
  esac
  case "$providers" in
    *grok*) CC_MODEL_HINTS+="grok-4.5 composer-2.5 grok composer " ;;
  esac
  export CC_MODEL_HINTS="${CC_MODEL_HINTS% }"
}

_inja_cc_list_profiles() {
  cat <<'EOF'
Claude Code provider combinations (any mix)

Named / combo profiles (separators: +  ,  -):
  claude              Claude only
  gpt                 ChatGPT / OpenAI only
  grok                Grok only  (opus/main → grok-4.5, fast → composer-2.5)
  multi               claude + gpt + grok
  gpt+grok            GPT + Grok (no Claude)
  claude+gpt          Claude + GPT
  claude+grok         Claude + Grok
  claude+gpt+grok     same as multi
  gpt,grok            same as gpt+grok

Slot defaults by combo:
  gpt only            opus=o3  sonnet=gpt  haiku=gpt-mini
  grok only           opus=grok-4.5  sonnet=grok-4.5  haiku=composer-2.5
  gpt+grok            opus=grok-4.5  sonnet=gpt  haiku=composer-2.5
  claude+gpt          opus=opus  sonnet=gpt  haiku=gpt-mini
  claude+grok         opus=opus  sonnet=grok-4.5  haiku=composer-2.5
  multi               opus=opus  sonnet=gpt  haiku=composer-2.5

In session (aliases from gateway.yaml):
  /model grok-4.5 | /model composer-2.5 | /model gpt | /model sonnet

Overrides:
  CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_MODEL
  CC_GROK_HEAVY=grok-4.5  CC_GROK_FAST=composer-2.5
  CC_GPT_HEAVY=o3  CC_GPT_MID=gpt  CC_GPT_FAST=gpt-mini
  CC_PROVIDERS=gpt,grok

Examples:
  ./examples/claude-code-multi.sh gpt
  ./examples/claude-code-multi.sh grok
  ./examples/claude-code-multi.sh gpt+grok
  ./examples/claude-code-multi.sh claude+grok --print "hi"
  CC_MODEL=composer-2.5 ./examples/claude-code-multi.sh grok
EOF
}
