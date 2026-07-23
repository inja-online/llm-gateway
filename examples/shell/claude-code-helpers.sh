# shellcheck shell=bash
# Sourceable helpers: Claude Code → Inja LLM Gateway (any provider combo).
#
# Works in bash and zsh:
#   source examples/shell/claude-code-helpers.sh
#   cc-gateway-up          # background HTTPS gateway + wait for healthz
#   cc-gpt                 # GPT only (Claude Code)
#   cc-grok                # Grok only
#   cc-gpt-grok            # GPT + Grok
#   cc-multi               # all three
#   cc-run gpt+grok
#   cc-list
#   cc-gateway-down
#
# Optional: KEY GATEWAY GATEWAY_CONFIG CC_*  INJA_GATEWAY_ROOT

# ---------------------------------------------------------------------------
# Resolve paths at *source* time (zsh-safe; do not rely on declare -F).
# ---------------------------------------------------------------------------
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  _INJA_SHELL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
elif [[ -n "${ZSH_VERSION:-}" ]]; then
  # shellcheck disable=SC2296
  _INJA_SHELL_DIR="$(cd "$(dirname "${(%):-%x}")" && pwd)"
else
  _INJA_SHELL_DIR="$(pwd)/examples/shell"
fi
_INJA_REPO_ROOT="$(cd "$_INJA_SHELL_DIR/../.." && pwd)"
if [[ -n "${INJA_GATEWAY_ROOT:-}" ]]; then
  _INJA_REPO_ROOT="$INJA_GATEWAY_ROOT"
fi

# Always load profiles from the same directory (fixes zsh "command not found").
# shellcheck source=claude-code-profiles.sh
source "$_INJA_SHELL_DIR/claude-code-profiles.sh"

_inja_gateway_root() {
  printf '%s' "${INJA_GATEWAY_ROOT:-$_INJA_REPO_ROOT}"
}

_inja_cc_state_dir() {
  printf '%s' "${INJA_GATEWAY_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/inja-gateway}"
}

_inja_cc_certs_dir() {
  printf '%s' "$(_inja_gateway_root)/examples/certs"
}

_inja_cc_default_config() {
  local root
  root="$(_inja_gateway_root)"
  if [[ -n "${GATEWAY_CONFIG:-}" ]]; then
    printf '%s' "$GATEWAY_CONFIG"
    return
  fi
  if [[ -f "$root/examples/configs/claude-code-subscriptions.yaml" ]]; then
    printf '%s' "$root/examples/configs/claude-code-subscriptions.yaml"
  else
    printf '%s' "$root/gateway.yaml"
  fi
}

_inja_cc_find_bin() {
  local root bin
  root="$(_inja_gateway_root)"
  if [[ -n "${LLM_GATEWAY_BIN:-}" && -x "${LLM_GATEWAY_BIN}" ]]; then
    printf '%s' "$LLM_GATEWAY_BIN"
    return
  fi
  if command -v llm-gateway >/dev/null 2>&1; then
    command -v llm-gateway
    return
  fi
  for bin in "$root/llm-gateway" "$root/gateway"; do
    if [[ -x "$bin" ]]; then
      printf '%s' "$bin"
      return
    fi
  done
  return 1
}

_inja_cc_ensure_tls() {
  local certs cert key
  certs="$(_inja_cc_certs_dir)"
  cert="$certs/localhost.pem"
  key="$certs/localhost-key.pem"
  if [[ -f "$cert" && -f "$key" ]]; then
    return 0
  fi
  local gen="$(_inja_gateway_root)/examples/scripts/gen-localhost-tls.sh"
  if [[ ! -x "$gen" ]]; then
    chmod +x "$gen" 2>/dev/null || true
  fi
  if [[ ! -f "$gen" ]]; then
    echo "missing $gen — cannot create TLS certs" >&2
    return 1
  fi
  bash "$gen" "$certs"
}

_inja_cc_public_base() {
  # Prefer explicit GATEWAY; else HTTPS localhost when certs exist.
  if [[ -n "${GATEWAY:-}" ]]; then
    printf '%s' "$GATEWAY"
    return
  fi
  local certs
  certs="$(_inja_cc_certs_dir)"
  if [[ -f "$certs/localhost.pem" && -f "$certs/localhost-key.pem" ]]; then
    printf 'https://127.0.0.1:8787'
  else
    printf 'http://127.0.0.1:8787'
  fi
}

_inja_cc_export_trust() {
  # Make Node/Claude Code accept self-signed (mkcert already system-trusted).
  local cert
  cert="$(_inja_cc_certs_dir)/localhost.pem"
  if [[ -f "$cert" ]]; then
    export NODE_EXTRA_CA_CERTS="$cert"
    export SSL_CERT_FILE="${SSL_CERT_FILE:-$cert}"
    export NODE_OPTIONS="${NODE_OPTIONS:-}"
  fi
}

# ---------------------------------------------------------------------------
# Background gateway lifecycle
# ---------------------------------------------------------------------------

cc-gateway-up() {
  local root bin cfg state pidfile logfile certs cert key
  root="$(_inja_gateway_root)"
  bin="$(_inja_cc_find_bin)" || {
    echo "llm-gateway not found. Build: (cd $root && go build -o llm-gateway ./cmd/gateway)" >&2
    return 127
  }
  cfg="$(_inja_cc_default_config)"
  state="$(_inja_cc_state_dir)"
  mkdir -p "$state"
  pidfile="$state/gateway.pid"
  logfile="$state/gateway.log"

  if [[ -f "$pidfile" ]]; then
    local old
    old="$(cat "$pidfile" 2>/dev/null || true)"
    if [[ -n "$old" ]] && kill -0 "$old" 2>/dev/null; then
      echo "gateway already running (pid $old)  log=$logfile" >&2
      _inja_cc_wire_client_env
      return 0
    fi
    rm -f "$pidfile"
  fi

  _inja_cc_ensure_tls || return $?
  certs="$(_inja_cc_certs_dir)"
  cert="$certs/localhost.pem"
  key="$certs/localhost-key.pem"

  export GATEWAY_CONFIG="$cfg"
  export GATEWAY_LISTEN="${GATEWAY_LISTEN:-127.0.0.1:8787}"
  export GATEWAY_TLS_CERT="$cert"
  export GATEWAY_TLS_KEY="$key"

  echo "starting $bin (HTTPS) config=$cfg" >&2
  # Detach fully so Claude Code sessions keep the gateway alive.
  nohup "$bin" -config "$cfg" >>"$logfile" 2>&1 &
  echo $! >"$pidfile"
  disown 2>/dev/null || true

  local i url
  url="https://127.0.0.1:8787"
  for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if curl -skf --max-time 1 "$url/healthz" >/dev/null 2>&1; then
      echo "gateway up  pid=$(cat "$pidfile")  $url/healthz  log=$logfile" >&2
      _inja_cc_wire_client_env
      return 0
    fi
    sleep 0.25
  done
  echo "gateway failed to become healthy — see $logfile" >&2
  tail -n 40 "$logfile" >&2 || true
  return 1
}

cc-gateway-down() {
  local state pidfile pid
  state="$(_inja_cc_state_dir)"
  pidfile="$state/gateway.pid"
  if [[ ! -f "$pidfile" ]]; then
    echo "no pid file ($pidfile)" >&2
    return 0
  fi
  pid="$(cat "$pidfile")"
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    sleep 0.3
    kill -9 "$pid" 2>/dev/null || true
    echo "stopped gateway pid $pid" >&2
  else
    echo "stale pid $pid" >&2
  fi
  rm -f "$pidfile"
}

cc-gateway-status() {
  local state pidfile pid url
  state="$(_inja_cc_state_dir)"
  pidfile="$state/gateway.pid"
  url="$(_inja_cc_public_base)"
  if [[ -f "$pidfile" ]]; then
    pid="$(cat "$pidfile")"
    if kill -0 "$pid" 2>/dev/null; then
      echo "running pid=$pid"
    else
      echo "pidfile stale pid=$pid"
    fi
  else
    echo "not managed (no pidfile)"
  fi
  if curl -skf --max-time 2 "$url/healthz" >/dev/null 2>&1; then
    echo "healthz ok  $url"
  else
    echo "healthz fail  $url"
  fi
}

# Export env Claude Code needs to talk to a running (HTTPS) gateway.
_inja_cc_wire_client_env() {
  export GATEWAY="$(_inja_cc_public_base)"
  export KEY="${KEY:-${GATEWAY_EDGE_KEY:-local-dev}}"
  _inja_cc_export_trust
  export ANTHROPIC_BASE_URL="$GATEWAY"
  export ANTHROPIC_API_KEY="$KEY"
  export ANTHROPIC_AUTH_TOKEN="${ANTHROPIC_AUTH_TOKEN:-$KEY}"
}

# ---------------------------------------------------------------------------
# Profile + Claude Code launch
# ---------------------------------------------------------------------------

_inja_cc_apply() {
  _inja_cc_wire_client_env
  _inja_cc_apply_combo "${1:-multi}"
}

cc-gateway-env() {
  local profile="${1:-${CC_GATEWAY_PROFILE:-multi}}"
  _inja_cc_apply "$profile" || return $?
  cat <<EOF
# profile=$CC_GATEWAY_PROFILE  providers=$CC_GATEWAY_PROVIDERS
export GATEWAY='$GATEWAY'
export KEY='$KEY'
export ANTHROPIC_BASE_URL='$ANTHROPIC_BASE_URL'
export ANTHROPIC_API_KEY='$ANTHROPIC_API_KEY'
export ANTHROPIC_AUTH_TOKEN='$ANTHROPIC_AUTH_TOKEN'
export ANTHROPIC_MODEL='$ANTHROPIC_MODEL'
export ANTHROPIC_DEFAULT_OPUS_MODEL='$ANTHROPIC_DEFAULT_OPUS_MODEL'
export ANTHROPIC_DEFAULT_SONNET_MODEL='$ANTHROPIC_DEFAULT_SONNET_MODEL'
export ANTHROPIC_DEFAULT_HAIKU_MODEL='$ANTHROPIC_DEFAULT_HAIKU_MODEL'
export ANTHROPIC_SMALL_FAST_MODEL='$ANTHROPIC_SMALL_FAST_MODEL'
export NODE_EXTRA_CA_CERTS='${NODE_EXTRA_CA_CERTS:-}'
export SSL_CERT_FILE='${SSL_CERT_FILE:-}'
EOF
}

# Foreground start (legacy name)
cc-gateway-start() {
  local root bin cfg
  root="$(_inja_gateway_root)"
  bin="$(_inja_cc_find_bin)" || {
    echo "llm-gateway not found" >&2
    return 127
  }
  _inja_cc_ensure_tls || return $?
  cfg="$(_inja_cc_default_config)"
  export GATEWAY_TLS_CERT="$(_inja_cc_certs_dir)/localhost.pem"
  export GATEWAY_TLS_KEY="$(_inja_cc_certs_dir)/localhost-key.pem"
  export GATEWAY_LISTEN="${GATEWAY_LISTEN:-127.0.0.1:8787}"
  echo "starting $bin -config $cfg (HTTPS foreground)" >&2
  exec "$bin" -config "$cfg"
}

_inja_cc_run() {
  local profile="$1"
  shift
  # Ensure gateway is up in background (HTTPS) before launching Claude Code.
  if [[ "${CC_SKIP_GATEWAY_UP:-}" != "1" ]]; then
    cc-gateway-up || return $?
  else
    _inja_cc_wire_client_env
  fi
  _inja_cc_apply_combo "$profile" || return $?
  if ! command -v claude >/dev/null 2>&1; then
    echo "claude CLI not found on PATH" >&2
    return 127
  fi
  echo "profile=$CC_GATEWAY_PROFILE  providers=$CC_GATEWAY_PROVIDERS  $ANTHROPIC_BASE_URL  model=$ANTHROPIC_MODEL" >&2
  echo "  /model $CC_MODEL_HINTS" >&2
  claude "$@"
}

cc-claude() { _inja_cc_run claude "$@"; }
cc-gpt()    { _inja_cc_run gpt "$@"; }
cc-grok()   { _inja_cc_run grok "$@"; }
cc-multi()  { _inja_cc_run multi "$@"; }
cc-gpt-grok()    { _inja_cc_run gpt+grok "$@"; }
cc-claude-gpt()  { _inja_cc_run claude+gpt "$@"; }
cc-claude-grok() { _inja_cc_run claude+grok "$@"; }

cc-run() {
  if [[ $# -lt 1 ]]; then
    echo "usage: cc-run <combo> [claude args...]" >&2
    _inja_cc_list_profiles >&2
    return 2
  fi
  local combo="$1"
  shift
  _inja_cc_run "$combo" "$@"
}

cc-list() { _inja_cc_list_profiles; }

ccg()  { cc-multi "$@"; }
ccgo() { cc-gpt "$@"; }
ccgx() { cc-grok "$@"; }
ccga() { cc-claude "$@"; }
ccgg() { cc-gpt-grok "$@"; }
