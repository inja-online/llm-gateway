# shellcheck shell=bash
# Multi-app integrations + switchable gateway/default profiles.
#
#   source examples/shell/claude-code-helpers.sh
#   source examples/shell/apps-helpers.sh
#   export KEY=local-dev
#   cc-gateway-up
#
# Switch (all managed apps):
#   apps-use-gateway          # snapshot "default" once, write gateway configs
#   apps-use-default          # restore pre-gateway / vendor settings
#   apps-switch gateway|default
#   apps-status
#   apps-list-backups
#   apps-backup [default|gateway]   # force-save current live → named slot
#   apps-rollback [default]         # alias for apps-use-default
#
# Per-app writes still backup into the profile store + history:
#   apps-write-claude-desktop | apps-write-claude-settings | apps-write-codex

if ! command -v _inja_gateway_root >/dev/null 2>&1; then
  if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
    _INJA_SHELL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  elif [[ -n "${ZSH_VERSION:-}" ]]; then
    # shellcheck disable=SC2296
    _INJA_SHELL_DIR="$(cd "$(dirname "${(%):-%x}")" && pwd)"
  else
    _INJA_SHELL_DIR="$(pwd)/examples/shell"
  fi
  # shellcheck source=claude-code-helpers.sh
  source "$_INJA_SHELL_DIR/claude-code-helpers.sh"
fi

# ---------------------------------------------------------------------------
# Paths / bases
# ---------------------------------------------------------------------------

_apps_base_anthropic() {
  local g
  g="${GATEWAY:-$(_inja_cc_public_base 2>/dev/null || echo 'https://127.0.0.1:8787')}"
  printf '%s' "${g%/}"
}

_apps_base_openai() {
  local g
  g="$(_apps_base_anthropic)"
  if [[ "$g" == */v1 ]]; then
    printf '%s' "$g"
  else
    printf '%s/v1' "$g"
  fi
}

_apps_key() {
  printf '%s' "${KEY:-${GATEWAY_EDGE_KEY:-local-dev}}"
}

_apps_cert() {
  printf '%s' "$(_inja_cc_certs_dir 2>/dev/null)/localhost.pem"
}

_apps_templates() {
  printf '%s/examples/apps' "$(_inja_gateway_root)"
}

_apps_claude_desktop_path() {
  case "$(uname -s 2>/dev/null)" in
    Darwin)
      printf '%s/Library/Application Support/Claude/claude_desktop_config.json' "$HOME"
      ;;
    Linux)
      printf '%s/.config/Claude/claude_desktop_config.json' "$HOME"
      ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
      printf '%s/Claude/claude_desktop_config.json' "${APPDATA:-$HOME/AppData/Roaming}"
      ;;
    *)
      printf '%s/.config/Claude/claude_desktop_config.json' "$HOME"
      ;;
  esac
}

_apps_claude_settings_path() {
  printf '%s/.claude/settings.json' "$HOME"
}

_apps_codex_path() {
  printf '%s/.codex/config.toml' "$HOME"
}

# Profile store: named slots default|gateway + history timestamps
_apps_profiles_dir() {
  if [[ -n "${INJA_APPS_PROFILES_DIR:-}" ]]; then
    printf '%s' "$INJA_APPS_PROFILES_DIR"
    return
  fi
  if command -v _inja_cc_state_dir >/dev/null 2>&1; then
    printf '%s/app-profiles' "$(_inja_cc_state_dir)"
  else
    printf '%s/.local/state/inja-gateway/app-profiles' "$HOME"
  fi
}

_apps_active_file() {
  printf '%s/active' "$(_apps_profiles_dir)"
}

# Managed targets: name → live path + snapshot filename
# Never use local variable named `path` (zsh PATH alias).
_apps_targets() {
  # space-separated: id
  printf '%s\n' claude-settings claude-desktop codex
}

_apps_target_live() {
  case "$1" in
    claude-settings) _apps_claude_settings_path ;;
    claude-desktop)  _apps_claude_desktop_path ;;
    codex)           _apps_codex_path ;;
    *) return 1 ;;
  esac
}

_apps_target_snap_name() {
  case "$1" in
    claude-settings) printf 'claude-settings.json' ;;
    claude-desktop)  printf 'claude-desktop.json' ;;
    codex)           printf 'codex.toml' ;;
    *) return 1 ;;
  esac
}

_apps_slot_dir() {
  local slot="$1"
  printf '%s/%s' "$(_apps_profiles_dir)" "$slot"
}

_apps_ensure_profiles() {
  mkdir -p "$(_apps_profiles_dir)/default" "$(_apps_profiles_dir)/gateway" "$(_apps_profiles_dir)/history"
}

_apps_set_active() {
  _apps_ensure_profiles
  printf '%s\n' "$1" >"$(_apps_active_file)"
}

_apps_get_active() {
  local f
  f="$(_apps_active_file)"
  if [[ -f "$f" ]]; then
    tr -d '[:space:]' <"$f"
  else
    printf 'unknown'
  fi
}

# Snapshot one live target into slot (default|gateway|history/TS)
# Marks missing live files with .missing sentinel so rollback can delete them.
_apps_snapshot_target() {
  local target="$1" slot="$2"
  local live snapdir snap miss
  live="$(_apps_target_live "$target")" || return 1
  snapdir="$(_apps_slot_dir "$slot")"
  snap="$snapdir/$(_apps_target_snap_name "$target")"
  miss="${snap}.missing"
  mkdir -p "$snapdir"
  rm -f "$miss"
  if [[ -f "$live" ]]; then
    cp "$live" "$snap"
    echo "  snapshot $target → $slot ($live)" >&2
  else
    rm -f "$snap"
    : >"$miss"
    echo "  snapshot $target → $slot (missing live file; noted)" >&2
  fi
}

_apps_snapshot_all() {
  local slot="$1" t
  _apps_ensure_profiles
  for t in $(_apps_targets); do
    _apps_snapshot_target "$t" "$slot"
  done
}

# History copy of current live (timestamped, never overwrites slots)
_apps_history_stamp() {
  date +%Y%m%d%H%M%S
}

_apps_snapshot_history() {
  local stamp label
  stamp="$(_apps_history_stamp)"
  label="${1:-manual}"
  _apps_snapshot_all "history/${stamp}-${label}"
  echo "history → $(_apps_profiles_dir)/history/${stamp}-${label}" >&2
}

# Restore one target from slot to live path
_apps_restore_target() {
  local target="$1" slot="$2"
  local live snapdir snap miss dir
  live="$(_apps_target_live "$target")" || return 1
  snapdir="$(_apps_slot_dir "$slot")"
  snap="$snapdir/$(_apps_target_snap_name "$target")"
  miss="${snap}.missing"
  dir="$(dirname "$live")"
  mkdir -p "$dir"

  if [[ -f "$miss" ]] || [[ ! -f "$snap" && ! -f "$live" ]]; then
    if [[ -f "$live" ]]; then
      rm -f "$live"
      echo "  restore $target ← $slot (removed; was missing pre-gateway)" >&2
    else
      echo "  restore $target ← $slot (still missing)" >&2
    fi
    return 0
  fi

  if [[ ! -f "$snap" ]]; then
    echo "  restore $target ← $slot FAILED: no snapshot at $snap" >&2
    return 1
  fi

  # Sidecar next to live file for last-second undo
  if [[ -f "$live" ]]; then
    cp "$live" "${live}.bak.$(_apps_history_stamp)"
  fi
  cp "$snap" "$live"
  echo "  restore $target ← $slot → $live" >&2
}

_apps_restore_all() {
  local slot="$1" t rc=0
  for t in $(_apps_targets); do
    _apps_restore_target "$t" "$slot" || rc=1
  done
  return "$rc"
}

# Save default only if slot empty (first enable)
_apps_ensure_default_snapshot() {
  local slotdir snap any=0 t
  slotdir="$(_apps_slot_dir default)"
  for t in $(_apps_targets); do
    snap="$slotdir/$(_apps_target_snap_name "$t")"
    if [[ -f "$snap" || -f "${snap}.missing" ]]; then
      any=1
      break
    fi
  done
  if [[ "$any" -eq 0 ]]; then
    echo "saving current live configs as profile 'default' (one-time)…" >&2
    _apps_snapshot_all default
  else
    echo "profile 'default' already present (use apps-backup default to refresh)" >&2
  fi
}

# ---------------------------------------------------------------------------
# Write gateway content for each target
# ---------------------------------------------------------------------------

_apps_write_gateway_claude_settings() {
  local live anth key cert
  live="$(_apps_claude_settings_path)"
  anth="$(_apps_base_anthropic)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"
  mkdir -p "$(dirname "$live")"
  cat >"$live" <<EOF
{
  "env": {
    "ANTHROPIC_BASE_URL": "$anth",
    "ANTHROPIC_API_KEY": "$key",
    "ANTHROPIC_AUTH_TOKEN": "$key",
    "ANTHROPIC_MODEL": "sonnet",
    "NODE_EXTRA_CA_CERTS": "$cert"
  }
}
EOF
  echo "wrote $live (gateway)" >&2
}

_apps_write_gateway_claude_desktop() {
  local live anth key cert
  live="$(_apps_claude_desktop_path)"
  anth="$(_apps_base_anthropic)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"
  mkdir -p "$(dirname "$live")"
  cat >"$live" <<EOF
{
  "mcpServers": {},
  "env": {
    "ANTHROPIC_BASE_URL": "$anth",
    "ANTHROPIC_API_KEY": "$key",
    "ANTHROPIC_AUTH_TOKEN": "$key",
    "ANTHROPIC_MODEL": "sonnet",
    "NODE_EXTRA_CA_CERTS": "$cert"
  }
}
EOF
  echo "wrote $live (gateway)" >&2
}

_apps_write_gateway_codex() {
  local live oai key
  live="$(_apps_codex_path)"
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  mkdir -p "$(dirname "$live")"
  cat >"$live" <<EOF
# Generated by apps-use-gateway — Inja LLM Gateway
# export INJA_GATEWAY_KEY=$key
# export NODE_EXTRA_CA_CERTS=$(_apps_cert)

model = "gpt"
model_provider = "inja"
openai_base_url = "$oai"

[model_providers.inja]
name = "Inja LLM Gateway"
base_url = "$oai"
env_key = "INJA_GATEWAY_KEY"
wire_api = "chat_completions"
# wire_api = "responses"
EOF
  echo "wrote $live (gateway)" >&2
  echo "  shell: export INJA_GATEWAY_KEY=$key" >&2
  echo "  shell: export NODE_EXTRA_CA_CERTS=$(_apps_cert)" >&2
}

_apps_write_all_gateway() {
  _apps_write_gateway_claude_settings
  _apps_write_gateway_claude_desktop
  _apps_write_gateway_codex
}

# ---------------------------------------------------------------------------
# Public: backup / switch / status
# ---------------------------------------------------------------------------

apps-backup() {
  local slot="${1:-default}"
  case "$slot" in
    default|gateway) ;;
    *)
      echo "usage: apps-backup [default|gateway]" >&2
      echo "  Saves current live configs into the named profile slot." >&2
      return 1
      ;;
  esac
  echo "apps-backup → profile '$slot'" >&2
  _apps_snapshot_history "pre-backup-$slot"
  _apps_snapshot_all "$slot"
  echo "done. profiles: $(_apps_profiles_dir)" >&2
}

apps-list-backups() {
  local root
  root="$(_apps_profiles_dir)"
  echo "Profile store: $root"
  echo "Active: $(_apps_get_active)"
  echo
  if [[ ! -d "$root" ]]; then
    echo "(empty — run apps-use-gateway or apps-backup first)"
    return 0
  fi
  echo "Slots:"
  for s in default gateway; do
    if [[ -d "$root/$s" ]]; then
      echo "  $s/"
      # shellcheck disable=SC2012
      ls -la "$root/$s" 2>/dev/null | sed 's/^/    /' || true
    else
      echo "  $s/  (none)"
    fi
  done
  if [[ -d "$root/history" ]]; then
    echo "History (newest last):"
    # shellcheck disable=SC2012
    ls -1 "$root/history" 2>/dev/null | tail -20 | sed 's/^/  /' || echo "  (none)"
  fi
}

apps-status() {
  local active t live snap def gway
  active="$(_apps_get_active)"
  cat <<EOF
── App config status ─────────────────────────────────────────────
Active profile:  $active
Profile store:   $(_apps_profiles_dir)

Live files:
EOF
  for t in $(_apps_targets); do
    live="$(_apps_target_live "$t")"
    if [[ -f "$live" ]]; then
      printf '  %-16s present  %s\n' "$t" "$live"
    else
      printf '  %-16s missing  %s\n' "$t" "$live"
    fi
  done
  echo
  echo "Named slots:"
  for t in $(_apps_targets); do
    snap="$(_apps_target_snap_name "$t")"
    def="$(_apps_slot_dir default)/$snap"
    gway="$(_apps_slot_dir gateway)/$snap"
    printf '  %-16s default=%s  gateway=%s\n' "$t" \
      "$([[ -f "$def" ]] && echo yes || ([[ -f "${def}.missing" ]] && echo 'missing-ok' || echo no))" \
      "$([[ -f "$gway" ]] && echo yes || ([[ -f "${gway}.missing" ]] && echo 'missing-ok' || echo no))"
  done
  cat <<EOF

Switch:
  apps-use-gateway     # llm-gateway configs (saves default on first run)
  apps-use-default     # restore pre-gateway settings
  apps-switch gateway|default
  apps-backup default  # refresh the "default" snapshot from live
  apps-list-backups
EOF
}

apps-use-gateway() {
  local anth oai key
  anth="$(_apps_base_anthropic)"
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"

  echo "══ apps-use-gateway ══" >&2
  echo "Anthropic base: $anth" >&2
  echo "OpenAI base:    $oai" >&2
  echo "Edge key:       $key" >&2
  echo >&2

  _apps_ensure_profiles
  _apps_snapshot_history "pre-gateway"
  _apps_ensure_default_snapshot

  # If currently on gateway, refresh gateway slot after write; if on default, we already snapshotted history
  _apps_write_all_gateway
  _apps_snapshot_all gateway
  _apps_set_active gateway

  cat <<EOF >&2

Active profile → gateway
Restart apps (fully quit Claude Desktop / Codex) so they reload config.

Shell for Codex:
  export INJA_GATEWAY_KEY=$key
  export NODE_EXTRA_CA_CERTS=$(_apps_cert)

Rollback anytime:
  apps-use-default
EOF
}

apps-use-default() {
  local slotdir
  slotdir="$(_apps_slot_dir default)"
  if [[ ! -d "$slotdir" ]]; then
    echo "No 'default' profile yet." >&2
    echo "Nothing to restore. (Run apps-use-gateway once to create default from pre-gateway files," >&2
    echo " or apps-backup default while on vendor settings.)" >&2
    return 1
  fi

  echo "══ apps-use-default (rollback) ══" >&2
  _apps_snapshot_history "pre-default"
  # Preserve current gateway live so we can flip back without regenerating
  if [[ "$(_apps_get_active)" == gateway ]] || [[ -f "$(_apps_claude_settings_path)" ]]; then
    echo "saving current live as profile 'gateway'…" >&2
    _apps_snapshot_all gateway
  fi

  if ! _apps_restore_all default; then
    echo "restore finished with errors — check apps-list-backups" >&2
  fi
  _apps_set_active default

  cat <<EOF >&2

Active profile → default
Restart apps so they drop gateway base URLs.

Re-enable gateway later:
  apps-use-gateway
  # or restore last gateway snapshot without rewriting:
  apps-restore-slot gateway
EOF
}

apps-restore-slot() {
  local slot="${1:-}"
  if [[ -z "$slot" || ( "$slot" != default && "$slot" != gateway ) ]]; then
    # allow history/TIMESTAMP
    if [[ -z "$slot" || ! -d "$(_apps_slot_dir "$slot")" ]]; then
      echo "usage: apps-restore-slot default|gateway|history/<stamp>" >&2
      apps-list-backups
      return 1
    fi
  fi
  if [[ ! -d "$(_apps_slot_dir "$slot")" ]]; then
    echo "slot not found: $(_apps_slot_dir "$slot")" >&2
    return 1
  fi
  echo "══ apps-restore-slot $slot ══" >&2
  _apps_snapshot_history "pre-restore-${slot//\//-}"
  _apps_restore_all "$slot"
  case "$slot" in
    default|gateway) _apps_set_active "$slot" ;;
    *) _apps_set_active "restored:$slot" ;;
  esac
  echo "Active → $(_apps_get_active) — restart apps" >&2
}

apps-switch() {
  case "${1:-}" in
    gateway|on|gw)
      apps-use-gateway
      ;;
    default|off|vendor|rollback)
      apps-use-default
      ;;
    status)
      apps-status
      ;;
    *)
      cat <<EOF >&2
usage: apps-switch gateway|default

  apps-switch gateway   → apps-use-gateway
  apps-switch default   → apps-use-default (rollback)
  apps-switch status    → apps-status
EOF
      return 1
      ;;
  esac
}

apps-rollback() {
  apps-use-default "$@"
}

# ---------------------------------------------------------------------------
# Overview + per-app print helpers
# ---------------------------------------------------------------------------

apps-setup() {
  local anth oai key cert
  anth="$(_apps_base_anthropic)"
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"

  cat <<EOF

═══════════════════════════════════════════════════════════════════
  Use subscriptions in ANY app (Inja LLM Gateway)
═══════════════════════════════════════════════════════════════════

Shared:
  source examples/shell/claude-code-helpers.sh && export KEY=$key && cc-gateway-up
  source examples/shell/apps-helpers.sh

  Anthropic base:  $anth
  OpenAI base:     $oai
  Edge key:        $key
  TLS cert:        $cert
  Active profile:  $(_apps_get_active)
  Profile store:   $(_apps_profiles_dir)

Switch (recommended):
  apps-use-gateway       write gateway configs (auto-backup → profile default)
  apps-use-default       restore pre-gateway / vendor settings
  apps-switch gateway|default
  apps-status
  apps-list-backups
  apps-backup default    refresh default snapshot from current live files

Per-app (print only unless apps-write-*):
  apps-claude-desktop · apps-codex · apps-continue · apps-cline
  apps-aider · apps-windsurf · apps-generic
  apps-write-claude-settings · apps-write-claude-desktop · apps-write-codex

Claude Code / Cursor:
  cc-gpt / cc-grok / cc-multi · cursor-setup

Docs: https://inja-online.github.io/llm-gateway/guides/app-integrations/

═══════════════════════════════════════════════════════════════════
EOF
}

apps-claude-desktop() {
  local desktop_cfg settings anth key cert
  desktop_cfg="$(_apps_claude_desktop_path)"
  settings="$(_apps_claude_settings_path)"
  anth="$(_apps_base_anthropic)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"

  cat <<EOF

── Claude Desktop ────────────────────────────────────────────────
Desktop JSON:  $desktop_cfg
Settings:      $settings

Prefer switch commands (backup + rollback built-in):
  apps-use-gateway
  apps-use-default

Or write only Claude files:
  apps-write-claude-settings
  apps-write-claude-desktop

Manual env:
  ANTHROPIC_BASE_URL=$anth
  ANTHROPIC_API_KEY=$key
  NODE_EXTRA_CA_CERTS=$cert
EOF
}

# Per-app write: history + optional default, then gateway content for that file only
apps-write-claude-desktop() {
  echo "apps-write-claude-desktop" >&2
  _apps_snapshot_history "pre-write-claude-desktop"
  _apps_snapshot_target claude-desktop "history/last-claude-desktop"
  if [[ ! -f "$(_apps_slot_dir default)/claude-desktop.json" && ! -f "$(_apps_slot_dir default)/claude-desktop.json.missing" ]]; then
    _apps_snapshot_target claude-desktop default
  fi
  _apps_write_gateway_claude_desktop
  _apps_snapshot_target claude-desktop gateway
  echo "rollback this file: apps-restore-slot default  (or apps-use-default for all)" >&2
}

apps-write-claude-settings() {
  echo "apps-write-claude-settings" >&2
  _apps_snapshot_history "pre-write-claude-settings"
  if [[ ! -f "$(_apps_slot_dir default)/claude-settings.json" && ! -f "$(_apps_slot_dir default)/claude-settings.json.missing" ]]; then
    _apps_snapshot_target claude-settings default
  fi
  _apps_write_gateway_claude_settings
  _apps_snapshot_target claude-settings gateway
  echo "rollback: apps-use-default" >&2
}

apps-codex() {
  local oai key cert codex_cfg
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"
  codex_cfg="$(_apps_codex_path)"

  cat <<EOF

── ChatGPT Desktop / Codex ───────────────────────────────────────
Config: $codex_cfg

  apps-use-gateway / apps-write-codex
  export INJA_GATEWAY_KEY=$key
  export NODE_EXTRA_CA_CERTS=$cert
  codex

Rollback: apps-use-default
EOF
}

apps-write-codex() {
  echo "apps-write-codex" >&2
  _apps_snapshot_history "pre-write-codex"
  if [[ ! -f "$(_apps_slot_dir default)/codex.toml" && ! -f "$(_apps_slot_dir default)/codex.toml.missing" ]]; then
    _apps_snapshot_target codex default
  fi
  _apps_write_gateway_codex
  _apps_snapshot_target codex gateway
  echo "rollback: apps-use-default" >&2
}

apps-continue() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Continue.dev ──────────────────────────────────────────────────
Merge examples/apps/continue/config.yaml into ~/.continue/config.yaml
apiBase: $oai
apiKey:  $key
(Not managed by apps-use-gateway — edit Continue config manually.)
EOF
}

apps-cline() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Cline / Roo Code ──────────────────────────────────────────────
  "cline.openAiBaseUrl": "$oai",
  "cline.openAiApiKey": "$key",
  "cline.openAiModelId": "gpt"
Snippet: examples/apps/cline/vscode-settings.snippet.json
(Not managed by apps-use-gateway.)
EOF
}

apps-aider() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Aider ─────────────────────────────────────────────────────────
export OPENAI_API_BASE=$oai
export OPENAI_API_KEY=$key
aider --model openai/gpt
(Process env only — no apps-use-gateway file.)
EOF
}

apps-windsurf() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Windsurf / Cascade ────────────────────────────────────────────
Base URL: $oai
API Key:  $key
(Not managed by apps-use-gateway — set in Windsurf UI.)
EOF
}

apps-generic() {
  cat <<EOF

── Generic SDKs ──────────────────────────────────────────────────
source examples/apps/generic/openai.env
source examples/apps/generic/anthropic.env
# Session env only — does not touch Desktop/Codex profiles.
EOF
}

# Aliases
apps-help() { apps-setup "$@"; }
apps-list() { apps-setup "$@"; }
apps-chatgpt() { apps-codex "$@"; }
apps-gpt-desktop() { apps-codex "$@"; }
apps-enable() { apps-use-gateway "$@"; }
apps-disable() { apps-use-default "$@"; }
