# shellcheck shell=bash
# Multi-app integrations: Claude Desktop, Codex/ChatGPT, Continue, Cline, Aider, …
#
#   source examples/shell/claude-code-helpers.sh
#   source examples/shell/apps-helpers.sh
#   export KEY=local-dev
#   cc-gateway-up
#   apps-setup                 # overview + every app
#   apps-claude-desktop        # Desktop config path + JSON
#   apps-write-claude-desktop  # write Desktop config (backup first)
#   apps-write-claude-settings # write ~/.claude/settings.json env (shared CLI+Desktop)
#   apps-codex / apps-write-codex
#   apps-continue / apps-cline / apps-aider / apps-windsurf / apps-generic

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

_apps_backup() {
  # Note: never name locals `path` — zsh ties that to PATH.
  local f="$1"
  if [[ -f "$f" ]]; then
    local bak="${f}.bak.$(date +%Y%m%d%H%M%S)"
    cp "$f" "$bak"
    echo "backup → $bak" >&2
  fi
}

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

Shared (already running?):
  source examples/shell/claude-code-helpers.sh && export KEY=$key && cc-gateway-up

  Anthropic base (Claude Desktop / Claude Code):  $anth
  OpenAI base    (Cursor / Codex / Continue / …): $oai
  Edge / API key:                                 $key
  TLS cert (self-signed):                         $cert

Apps:
  apps-claude-desktop        Claude Desktop (claude_desktop_config.json)
  apps-write-claude-desktop  write Desktop config (backs up existing)
  apps-write-claude-settings write ~/.claude/settings.json (CLI + Desktop share)
  apps-codex / apps-write-codex
                             ChatGPT Desktop Codex / Codex CLI
  apps-continue              Continue.dev
  apps-cline                 Cline / Roo (VS Code settings)
  apps-aider                 Aider
  apps-windsurf              Windsurf / Cascade OpenAI-compatible
  apps-generic               OpenAI + Anthropic SDK env
  cursor-setup               Cursor IDE (source cursor-helpers.sh)
  cc-gpt / cc-grok / cc-multi  Claude Code combos

Models (short aliases — refresh with ?live=1):
  Claude:  sonnet opus haiku fable
  GPT:     gpt terra sol luna
  Grok:    grok-4.5 composer-2.5

Live catalog:
  curl -sk -H "Authorization: Bearer $key" '$oai/models?live=1' | jq -r '.data[].id'

Docs: https://inja-online.github.io/llm-gateway/guides/app-integrations/
Templates: $(_apps_templates)

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
Config paths (use either or both):

  Desktop JSON:  $desktop_cfg
  Shared settings (CLI + many Desktop builds):  $settings

1) Start gateway:  cc-gateway-up
2) Developer Mode → Edit Config, or edit settings.json.
3) Preferred: put env in ~/.claude/settings.json (shared with Claude Code):

{
  "env": {
    "ANTHROPIC_BASE_URL": "$anth",
    "ANTHROPIC_API_KEY": "$key",
    "ANTHROPIC_AUTH_TOKEN": "$key",
    "ANTHROPIC_MODEL": "sonnet",
    "NODE_EXTRA_CA_CERTS": "$cert"
  }
}

4) Desktop-only file (keep existing mcpServers when merging):

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

5) Fully quit and reopen Claude Desktop.
6) Prefer gateway aliases (sonnet / gpt / grok-4.5) when the UI allows custom models.

Write helpers:
  apps-write-claude-settings   # recommended first
  apps-write-claude-desktop    # Desktop JSON (minimal; re-add MCP if wiped)

Template: examples/apps/claude-desktop/claude_desktop_config.json

Note: Enterprise/managed builds may use inferenceGatewayBaseUrl instead of env.
EOF
}

apps-write-claude-desktop() {
  local desktop_cfg anth key cert
  desktop_cfg="$(_apps_claude_desktop_path)"
  anth="$(_apps_base_anthropic)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"
  mkdir -p "$(dirname "$desktop_cfg")"
  _apps_backup "$desktop_cfg"
  # Minimal write: does not merge complex MCP graphs — re-add MCP if needed.
  cat >"$desktop_cfg" <<EOF
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
  echo "wrote $desktop_cfg — fully quit and reopen Claude Desktop" >&2
}

apps-write-claude-settings() {
  local settings_cfg anth key cert
  settings_cfg="$(_apps_claude_settings_path)"
  anth="$(_apps_base_anthropic)"
  key="$(_apps_key)"
  cert="$(_apps_cert)"
  mkdir -p "$(dirname "$settings_cfg")"
  _apps_backup "$settings_cfg"
  # Overwrite with gateway env. Preserve nothing complex — user merges advanced keys manually.
  cat >"$settings_cfg" <<EOF
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
  echo "wrote $settings_cfg — restart Claude Desktop / Claude Code" >&2
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
(Codex CLI + many ChatGPT desktop coding modes)

export INJA_GATEWAY_KEY=$key
export NODE_EXTRA_CA_CERTS=$cert

# Append / merge into ~/.codex/config.toml:

model = "gpt"
model_provider = "inja"
openai_base_url = "$oai"

[model_providers.inja]
name = "Inja LLM Gateway"
base_url = "$oai"
env_key = "INJA_GATEWAY_KEY"
wire_api = "chat_completions"

# Models: gpt, sol, terra, luna, grok-4.5, sonnet
# Template: examples/apps/codex/config.toml
# Write: apps-write-codex

Then: codex

Caveat: pure ChatGPT chat UI may ignore custom providers.
Codex / coding-agent surfaces that read config.toml are supported.
EOF
}

apps-write-codex() {
  local codex_cfg oai key
  codex_cfg="$(_apps_codex_path)"
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  mkdir -p "$(dirname "$codex_cfg")"
  _apps_backup "$codex_cfg"
  cat >"$codex_cfg" <<EOF
# Generated by apps-write-codex — Inja LLM Gateway
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
# wire_api = "responses"  # try if your Codex build needs agent Responses
EOF
  echo "wrote $codex_cfg" >&2
  echo "export INJA_GATEWAY_KEY=$key" >&2
  echo "export NODE_EXTRA_CA_CERTS=$(_apps_cert)" >&2
}

apps-continue() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Continue.dev ──────────────────────────────────────────────────
Merge models from examples/apps/continue/config.yaml into ~/.continue/config.yaml

apiBase: $oai
apiKey:  $key
model:   gpt | sol | grok-4.5 | sonnet | …

Restart VS Code / Continue after edit.
EOF
}

apps-cline() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Cline / Roo Code ──────────────────────────────────────────────
VS Code / Cursor settings (names vary by extension version):

  "cline.apiProvider": "openai",
  "cline.openAiBaseUrl": "$oai",
  "cline.openAiApiKey": "$key",
  "cline.openAiModelId": "gpt"

Snippet: examples/apps/cline/vscode-settings.snippet.json
EOF
}

apps-aider() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Aider ─────────────────────────────────────────────────────────
# Shell (OpenAI-compatible → gateway):

export OPENAI_API_BASE=$oai
export OPENAI_API_KEY=$key
# or: export OPENAI_API_BASE / AIDER_OPENAI_API_BASE per your Aider version

aider --model openai/gpt
# or:  aider --openai-api-base $oai --openai-api-key $key --model gpt

Template env: examples/apps/aider/aider.env
Models: gpt, sol, grok-4.5, sonnet, …
EOF
}

apps-windsurf() {
  local oai key
  oai="$(_apps_base_openai)"
  key="$(_apps_key)"
  cat <<EOF

── Windsurf / Cascade ────────────────────────────────────────────
Settings → AI / Models → OpenAI-compatible (or custom provider):

  Base URL:  $oai
  API Key:   $key
  Model:     gpt | sol | grok-4.5 | sonnet | …

UI labels change by Windsurf version — look for "OpenAI Base URL",
"Custom provider", or "Override endpoint".

Snippet: examples/apps/windsurf/settings.snippet.md
EOF
}

apps-generic() {
  cat <<EOF

── Generic SDKs ──────────────────────────────────────────────────
OpenAI-compatible:
  source examples/apps/generic/openai.env

Anthropic Messages:
  source examples/apps/generic/anthropic.env

Python (openai):
  client = OpenAI(base_url="$(_apps_base_openai)", api_key="$(_apps_key)")

Python (anthropic):
  client = Anthropic(base_url="$(_apps_base_anthropic)", api_key="$(_apps_key)")

curl:
  curl -sk -H "Authorization: Bearer $(_apps_key)" \\
    -H "Content-Type: application/json" \\
    -d '{"model":"gpt","messages":[{"role":"user","content":"hi"}]}' \\
    "$(_apps_base_openai)/chat/completions"
EOF
}

# Aliases
apps-help() { apps-setup "$@"; }
apps-list() { apps-setup "$@"; }
apps-chatgpt() { apps-codex "$@"; }
apps-gpt-desktop() { apps-codex "$@"; }
