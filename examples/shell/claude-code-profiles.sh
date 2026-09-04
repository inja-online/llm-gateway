# shellcheck shell=bash
# Shared Claude Code profile resolver for any provider combination.
#
# Profiles (names or combos):
#   claude | gpt | grok | gemini
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
#   Gemini defaults: opus/sonnet/haiku → gemini aliases (live ids via CC_MODEL)
#
# Env overrides (always win):
#   CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_MODEL CC_SMALL_FAST_MODEL
#   CC_PROVIDERS=gpt,grok   # same as profile gpt+grok

# Normalize a profile string → sorted unique provider list (space-separated).
# Prints: "claude" | "gpt" | "grok" | "gemini" | "claude gpt" | ...
_inja_cc_normalize_providers() {
  local raw="${1:-}"
  # GNU tr treats '+- ' as a reverse range; keep '-' last so it is literal.
  raw="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]' | tr ',/| -' '+++++' | tr -s '+')"
  # named shortcuts
  case "$raw" in
    ""|multi|all|full|cgg|cgx) raw="claude+gpt+grok" ;;
    openai|chatgpt|codex) raw="gpt" ;;
    xai|supergrok) raw="grok" ;;
    anthropic) raw="claude" ;;
    google|antigravity) raw="gemini" ;;
    gpt+xai|xai+gpt|openai+grok|grok+openai) raw="gpt+grok" ;;
    claude+openai|openai+claude) raw="claude+gpt" ;;
    claude+xai|xai+claude) raw="claude+grok" ;;
  esac

  # Split on + without `read -a` (zsh rejects -a; bash uses -a / zsh uses -A).
  local has_c=0 has_g=0 has_x=0 has_m=0
  local rest="$raw" part
  while [[ -n "$rest" ]]; do
    case "$rest" in
      *'+'*)
        part="${rest%%+*}"
        rest="${rest#*+}"
        ;;
      *)
        part="$rest"
        rest=""
        ;;
    esac
    part="${part// /}"
    case "$part" in
      claude|anthropic) has_c=1 ;;
      gpt|openai|chatgpt|codex) has_g=1 ;;
      grok|xai|supergrok) has_x=1 ;;
      gemini|google|antigravity) has_m=1 ;;
      multi|all) has_c=1; has_g=1; has_x=1 ;;
      "") ;;
      *)
        echo "unknown provider in profile: $part (use claude|gpt|grok|gemini)" >&2
        return 2
        ;;
    esac
  done

  local out=""
  [[ $has_c -eq 1 ]] && out="${out}claude "
  [[ $has_g -eq 1 ]] && out="${out}gpt "
  [[ $has_x -eq 1 ]] && out="${out}grok "
  [[ $has_m -eq 1 ]] && out="${out}gemini "
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
  local has_c=0 has_g=0 has_x=0 has_m=0
  # Word-boundary-ish: space-padded match
  local pad=" $providers "
  [[ "$pad" == *" claude "* ]] && has_c=1
  [[ "$pad" == *" gpt "* ]] && has_g=1
  [[ "$pad" == *" grok "* ]] && has_x=1
  [[ "$pad" == *" gemini "* ]] && has_m=1

  local n=$((has_c + has_g + has_x))
  PROFILE_LABEL="${providers// /+}"

  # Defaults per provider family (gateway aliases)
  # Grok: heavy = grok-4.5, fast/coding = grok-build (Composer-class)
  local g_heavy="${CC_GROK_HEAVY:-grok-4.5}"
  local g_fast="${CC_GROK_FAST:-composer-2.5}"
  # GPT-5.6: Sol (flagship) / Terra (default) / Luna (fast)
  local gpt_heavy="${CC_GPT_HEAVY:-sol}"
  local gpt_mid="${CC_GPT_MID:-gpt}"
  local gpt_fast="${CC_GPT_FAST:-luna}"
  local c_opus="${CC_CLAUDE_OPUS:-opus}"
  local c_sonnet="${CC_CLAUDE_SONNET:-sonnet}"
  local c_haiku="${CC_CLAUDE_HAIKU:-haiku}"
  local gm_heavy="${CC_GEMINI_HEAVY:-gemini-pro}"
  local gm_mid="${CC_GEMINI_MID:-gemini}"
  local gm_fast="${CC_GEMINI_FAST:-gemini-flash}"

  if [[ $has_m -eq 1 && $has_c -eq 0 && $has_g -eq 0 && $has_x -eq 0 ]]; then
    OPUS_M="$gm_heavy"; SONNET_M="$gm_mid"; HAIKU_M="$gm_fast"; MAIN_M="$gm_mid"
  elif [[ $n -eq 1 ]]; then
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
    # all three (gemini extra does not change Claude/GPT/Grok slots)
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

  # Do not mark these local: _inja_cc_map_slots assigns them, and zsh local is
  # not dynamically scoped like bash.
  OPUS_M="" SONNET_M="" HAIKU_M="" MAIN_M="" PROFILE_LABEL=""
  _inja_cc_map_slots "$providers"

  local gateway
  if [[ -n "${GATEWAY:-}" ]]; then
    gateway="$GATEWAY"
  elif command -v _inja_cc_public_base >/dev/null 2>&1; then
    gateway="$(_inja_cc_public_base)"
  else
    gateway="https://127.0.0.1:8787"
  fi
  local key="${KEY:-${GATEWAY_EDGE_KEY:-${ANTHROPIC_API_KEY:-gateway}}}"

  export ANTHROPIC_BASE_URL="$gateway"
  export ANTHROPIC_AUTH_TOKEN="${ANTHROPIC_AUTH_TOKEN:-$key}"
  unset ANTHROPIC_API_KEY
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
    *gpt*) CC_MODEL_HINTS+="gpt sol terra luna " ;;
  esac
  case "$providers" in
    *grok*) CC_MODEL_HINTS+="grok-4.6 grok-4.5 composer-2.5 grok-build grok " ;;
  esac
  case "$providers" in
    *gemini*) CC_MODEL_HINTS+="gemini gemini-pro gemini-flash " ;;
  esac
  export CC_MODEL_HINTS="${CC_MODEL_HINTS% }"
  _inja_cc_apply_thinking
}


# Claude Code treats unknown ANTHROPIC_BASE_URL models as having no thinking /
# effort / ultracode. Advertise capabilities on every slot and write a session
# --settings file (modelPicker only — do not pin effort or ultracode; /effort
# and /model must stick). Ultracode is session-scoped; it does not persist in
# ~/.claude/settings.json. Opt in: CC_ULTRACODE=1.
_INJA_CC_THINKING_CAPS="${_INJA_CC_THINKING_CAPS:-effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking}"

_inja_cc_thinking_caps() {
  printf '%s' "${CC_THINKING_CAPS:-$_INJA_CC_THINKING_CAPS}"
}

_inja_cc_apply_thinking() {
  local caps
  caps="$(_inja_cc_thinking_caps)"
  # Do not default CLAUDE_CODE_EFFORT_LEVEL — that env pin overrides /effort
  # for the whole process. CLI --effort accepts low|medium|high|xhigh only.
  case "${CLAUDE_CODE_EFFORT_LEVEL:-}" in
    ultracode|ultra|max) export CLAUDE_CODE_EFFORT_LEVEL=xhigh ;;
  esac
  export CLAUDE_CODE_ALWAYS_ENABLE_EFFORT="${CLAUDE_CODE_ALWAYS_ENABLE_EFFORT:-1}"
  export ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES="${ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES:-$caps}"
  export ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES="${ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES:-$caps}"
  export ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES="${ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES:-$caps}"
  export ANTHROPIC_CUSTOM_MODEL_OPTION="${ANTHROPIC_CUSTOM_MODEL_OPTION:-$MAIN_M}"
  export ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES="${ANTHROPIC_CUSTOM_MODEL_OPTION_SUPPORTED_CAPABILITIES:-$caps}"
}

# Session settings: modelPicker behavesAs so unknown gateway aliases inherit
# Fable 5 client handling. replaceBuiltInOptions hides the Anthropic lineup
# (T3 / Claude Code otherwise show opus/sonnet/haiku next to gateway ids).
# No "model" / "effortLevel" keys — those pin /model and /effort across restart.
_inja_cc_write_ultracode_settings() {
  local f="${CC_ULTRACODE_SETTINGS:-}"
  if [[ -z "$f" ]]; then
    f="${XDG_STATE_HOME:-$HOME/.local/state}/inja-gateway/claude-code-ultracode.json"
  fi
  mkdir -p "$(dirname "$f")"
  local ultra_line=""
  if [[ "${CC_ULTRACODE:-}" == "1" ]]; then
    ultra_line='  "ultracode": true,'$'\n'
  fi
  cat >"$f" <<EOF
{
${ultra_line}  "alwaysThinkingEnabled": true,
  "skipWorkflowUsageWarning": true,
  "modelPicker": {
    "replaceBuiltInOptions": true,
    "options": [
      {"model": "sonnet", "label": "Sonnet (gateway)", "behavesAs": "claude-fable-5"},
      {"model": "opus", "label": "Opus (gateway)", "behavesAs": "claude-fable-5"},
      {"model": "haiku", "label": "Haiku (gateway)", "behavesAs": "claude-fable-5"},
      {"model": "fable", "label": "Fable (gateway)", "behavesAs": "claude-fable-5"},
      {"model": "claude", "label": "Claude (gateway)", "behavesAs": "claude-fable-5"},
      {"model": "gpt", "label": "GPT Terra", "behavesAs": "claude-fable-5"},
      {"model": "terra", "label": "Terra", "behavesAs": "claude-fable-5"},
      {"model": "sol", "label": "Sol", "behavesAs": "claude-fable-5"},
      {"model": "luna", "label": "Luna", "behavesAs": "claude-fable-5"},
      {"model": "gpt-5.6", "label": "GPT-5.6", "behavesAs": "claude-fable-5"},
      {"model": "gpt-heavy", "label": "GPT heavy", "behavesAs": "claude-fable-5"},
      {"model": "gpt-mini", "label": "GPT mini", "behavesAs": "claude-fable-5"},
      {"model": "grok-4.6", "label": "Grok 4.6", "behavesAs": "claude-fable-5"},
      {"model": "grok-4.5", "label": "Grok 4.5", "behavesAs": "claude-fable-5"},
      {"model": "grok", "label": "Grok", "behavesAs": "claude-fable-5"},
      {"model": "composer-2.5", "label": "Composer 2.5", "behavesAs": "claude-fable-5"},
      {"model": "composer", "label": "Composer", "behavesAs": "claude-fable-5"},
      {"model": "grok-build", "label": "Grok Build", "behavesAs": "claude-fable-5"},
      {"model": "gemini", "label": "Gemini", "behavesAs": "claude-fable-5"},
      {"model": "gemini-pro", "label": "Gemini Pro", "behavesAs": "claude-fable-5"},
      {"model": "gemini-flash", "label": "Gemini Flash", "behavesAs": "claude-fable-5"},
      {"model": "claude/sonnet", "label": "claude/sonnet", "behavesAs": "claude-fable-5"},
      {"model": "claude/opus", "label": "claude/opus", "behavesAs": "claude-fable-5"},
      {"model": "claude/haiku", "label": "claude/haiku", "behavesAs": "claude-fable-5"},
      {"model": "claude/fable-5", "label": "claude/fable-5", "behavesAs": "claude-fable-5"},
      {"model": "chatgpt/terra", "label": "chatgpt/terra", "behavesAs": "claude-fable-5"},
      {"model": "chatgpt/sol", "label": "chatgpt/sol", "behavesAs": "claude-fable-5"},
      {"model": "chatgpt/luna", "label": "chatgpt/luna", "behavesAs": "claude-fable-5"},
      {"model": "grok/4.6", "label": "grok/4.6", "behavesAs": "claude-fable-5"},
      {"model": "grok/4.5", "label": "grok/4.5", "behavesAs": "claude-fable-5"},
      {"model": "grok/composer-2.5", "label": "grok/composer-2.5", "behavesAs": "claude-fable-5"},
      {"model": "inja/sonnet", "label": "inja/sonnet", "behavesAs": "claude-fable-5"},
      {"model": "inja/sol", "label": "inja/sol", "behavesAs": "claude-fable-5"},
      {"model": "inja/grok-4.6", "label": "inja/grok-4.6", "behavesAs": "claude-fable-5"},
      {"model": "inja/gemini", "label": "inja/gemini", "behavesAs": "claude-fable-5"}
    ]
  }
}
EOF
  printf '%s' "$f"
}

# Populate CC_LAUNCH_EXTRA with --settings <picker json> unless the caller
# already passed --settings. Do not pass --effort unless the user set
# CLAUDE_CODE_EFFORT_LEVEL or passed --effort (env pin otherwise overrides /effort).
# Pass --model $ANTHROPIC_MODEL so a leftover user/project `model` pin
# (Claude /model "save as default") does not own this session. Env
# ANTHROPIC_MODEL alone does not silence `.claude/settings.json pins …`.
_inja_cc_prepare_claude_launch() {
  CC_LAUNCH_EXTRA=()
  local has_effort=0 has_settings=0 has_model=0 a
  for a in "$@"; do
    case "$a" in
      --effort|--effort=*) has_effort=1 ;;
      --settings|--settings=*) has_settings=1 ;;
      --model|--model=*) has_model=1 ;;
    esac
  done
  if [[ $has_effort -eq 0 && -n "${CLAUDE_CODE_EFFORT_LEVEL:-}" ]]; then
    CC_LAUNCH_EXTRA+=(--effort "$CLAUDE_CODE_EFFORT_LEVEL")
  fi
  if [[ $has_settings -eq 0 ]]; then
    local f
    f="$(_inja_cc_write_ultracode_settings)"
    CC_LAUNCH_EXTRA+=(--settings "$f")
  fi
  if [[ $has_model -eq 0 && -n "${ANTHROPIC_MODEL:-}" ]]; then
    CC_LAUNCH_EXTRA+=(--model "$ANTHROPIC_MODEL")
  fi
}

_inja_cc_list_profiles() {
  cat <<'EOF'
Claude Code provider combinations (any mix)

Named / combo profiles (separators: +  ,  -):
  claude              Claude only
  gpt                 ChatGPT / OpenAI / Codex only
  grok                Grok only  (main → grok-4.5, fast → grok-build-0.1 / Composer)
  gemini              Gemini / Antigravity only (main → gemini)
  multi               claude + gpt + grok
  gpt+grok            GPT + Grok (no Claude)
  claude+gpt          Claude + GPT
  claude+grok         Claude + Grok
  claude+gpt+grok     same as multi
  gpt,grok            same as gpt+grok

Slot defaults by combo (alias names → see examples/configs/* aliases, 2026-07):
  gpt only            opus=sol  sonnet=gpt(terra)  haiku=luna
  grok only           opus=grok-4.5  sonnet=grok-4.5  haiku=composer-2.5
  gemini only         opus=gemini-pro  sonnet=gemini  haiku=gemini-flash
  gpt+grok            opus=grok-4.5  sonnet=gpt  haiku=composer-2.5
  claude+gpt          opus=opus  sonnet=gpt  haiku=luna
  claude+grok         opus=opus  sonnet=grok-4.5  haiku=composer-2.5
  multi               opus=opus  sonnet=gpt  haiku=composer-2.5

Upstream targets (pinned in YAML):
  sonnet → claude-sonnet-5   opus → claude-opus-4-8   haiku → claude-haiku-4-5
  gpt/terra → gpt-5.6-terra  sol → gpt-5.6-sol  luna → gpt-5.6-luna
  grok-4.5 → grok-4.5        composer-2.5 → grok-build-0.1
  gemini → google/gemini-2.5-flash   gemini-pro → google/gemini-2.5-pro

In session:
  /model grok-4.6 | /model grok-4.5 | /model composer-2.5 | /model sol | /model terra | /model sonnet | /model gemini

Overrides:
  CC_OPUS_MODEL CC_SONNET_MODEL CC_HAIKU_MODEL CC_MODEL
  CC_GROK_HEAVY=grok-4.5  CC_GROK_FAST=composer-2.5
  CC_GPT_HEAVY=sol  CC_GPT_MID=gpt  CC_GPT_FAST=luna
  CC_GEMINI_HEAVY=gemini-pro  CC_GEMINI_MID=gemini  CC_GEMINI_FAST=gemini-flash
  CC_PROVIDERS=gpt,grok
EOF
}
