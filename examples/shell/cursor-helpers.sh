# shellcheck shell=bash
# Cursor IDE → Inja LLM Gateway (same ChatGPT / Claude / SuperGrok subscriptions).
#
#   source examples/shell/claude-code-helpers.sh
#   source examples/shell/cursor-helpers.sh
#   export KEY=local-dev
#   cc-gateway-up
#   cursor-setup
#   cursor-models
#   cursor-apply          # automate: write base URL + merge custom models into Cursor state
#   cursor-status         # show current Cursor openAIBaseUrl + userAddedModels
#   cursor-rollback       # restore last backup of Cursor applicationUser blob
#   cursor-verify
#
# Cursor built-ins stay; custom names use prefixes:
#   claude/fable-5  → gateway |  Claude Fable 5 → Cursor product

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

_cursor_base_url() {
  local g
  g="${GATEWAY:-$(_inja_cc_public_base 2>/dev/null || echo 'https://127.0.0.1:8787')}"
  g="${g%/}"
  if [[ "$g" == */v1 ]]; then
    printf '%s' "$g"
  else
    printf '%s/v1' "$g"
  fi
}

_cursor_key() {
  printf '%s' "${KEY:-${GATEWAY_EDGE_KEY:-local-dev}}"
}

# Recommended custom model names (prefixed — coexist with Cursor built-ins).
_cursor_gateway_models() {
  cat <<'EOF'
claude/fable-5
claude/fable
claude/sonnet-5
claude/sonnet
claude/opus
claude/opus-4-8
claude/haiku
claude/haiku-4-5
chatgpt/terra
chatgpt/sol
chatgpt/luna
chatgpt/gpt
grok/4.5
grok/grok-4.5
grok/composer-2.5
grok/build
inja/fable-5
inja/sonnet
inja/gpt
inja/sol
inja/grok-4.5
inja/composer-2.5
EOF
}

# Prefer command grep/rg — user often aliases grep→rg, and rg -E is "encoding".
_cursor_filter_prefixed() {
  if command -v jq >/dev/null 2>&1; then
    jq -r '.data[].id // empty' 2>/dev/null \
      | command grep -E '^(claude|chatgpt|grok|inja)/' 2>/dev/null \
      | command sort -u
  else
    command grep -oE '"(claude|chatgpt|grok|inja)/[^"]+"' 2>/dev/null \
      | tr -d '"' | command sort -u
  fi
}

_cursor_state_db() {
  if [[ -n "${CURSOR_STATE_DB:-}" ]]; then
    printf '%s' "$CURSOR_STATE_DB"
    return
  fi
  case "$(uname -s 2>/dev/null)" in
    Darwin)
      printf '%s/Library/Application Support/Cursor/User/globalStorage/state.vscdb' "$HOME"
      ;;
    Linux)
      printf '%s/.config/Cursor/User/globalStorage/state.vscdb' "$HOME"
      ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT)
      printf '%s/Cursor/User/globalStorage/state.vscdb' "${APPDATA:-$HOME/AppData/Roaming}"
      ;;
    *)
      printf '%s/.config/Cursor/User/globalStorage/state.vscdb' "$HOME"
      ;;
  esac
}

_cursor_backup_dir() {
  if command -v _inja_cc_state_dir >/dev/null 2>&1; then
    printf '%s/cursor-backups' "$(_inja_cc_state_dir)"
  else
    printf '%s/.local/state/inja-gateway/cursor-backups' "$HOME"
  fi
}

_cursor_appuser_key() {
  printf '%s' 'src.vs.platform.reactivestorage.browser.reactiveStorageServiceImpl.persistentStorage.applicationUser'
}

_cursor_is_running() {
  # Best-effort: main Cursor process (not helpers alone).
  if command -v pgrep >/dev/null 2>&1; then
    pgrep -x Cursor >/dev/null 2>&1 && return 0
    pgrep -f '/Cursor\.app/Contents/MacOS/Cursor' >/dev/null 2>&1 && return 0
    pgrep -f 'cursor.AppImage' >/dev/null 2>&1 && return 0
  fi
  return 1
}

cursor-models() {
  cat <<EOF
── Cursor custom models (llm-gateway) ────────────────────────────
Add each line in Settings → Models → Add Model
  — or run:  cursor-apply   (writes Cursor state.vscdb for you)

Leave Cursor's built-in models enabled (Claude Fable 5, Composer, …).

EOF
  _cursor_gateway_models
  cat <<EOF

Side-by-side example:
  Claude Fable 5     → Cursor product (Cursor billing)
  claude/fable-5     → Claude sub via llm-gateway
  Composer 2.5       → Cursor's Composer
  grok/composer-2.5  → SuperGrok Build via llm-gateway

Full list file: examples/cursor/models-to-add.txt
EOF
}

cursor-setup() {
  local base key cert
  base="$(_cursor_base_url)"
  key="$(_cursor_key)"
  cert="$(_inja_cc_certs_dir 2>/dev/null)/localhost.pem"

  cat <<EOF

═══════════════════════════════════════════════════════════════════
  Cursor → Inja LLM Gateway  (built-ins + gateway models together)
═══════════════════════════════════════════════════════════════════

Automated (recommended):
  1) Fully quit Cursor
  2) cc-gateway-up
  3) cursor-apply              # merges models + sets OpenAI base URL
  4) Reopen Cursor
  5) Settings → Models → paste OpenAI API Key once: $key
     (key is OS-encrypted; we cannot write it safely from shell)

Manual: cursor-models  then Add Model for each line.

  Cursor built-in          llm-gateway custom
  ─────────────────        ──────────────────
  Claude Fable 5           claude/fable-5
  Composer 2.5             grok/composer-2.5
  Cursor Grok 4.5          grok/4.5

Settings fields:
  OpenAI API Key:           $key
  Override OpenAI Base URL: $base

State DB: $(_cursor_state_db)
Backups:  $(_cursor_backup_dir)

TLS cert: $cert  (prefer mkcert -install)

  cursor-status    # what Cursor has now
  cursor-rollback  # restore previous applicationUser
  cursor-verify    # gateway catalog

Docs: https://inja-online.github.io/llm-gateway/guides/cursor-subscriptions/

═══════════════════════════════════════════════════════════════════
EOF
}

cursor-status() {
  local db
  db="$(_cursor_state_db)"
  if [[ ! -f "$db" ]]; then
    echo "Cursor state DB not found: $db" >&2
    return 1
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 required for cursor-status" >&2
    return 1
  fi
  CURSOR_STATE_DB="$db" CURSOR_APPUSER_KEY="$(_cursor_appuser_key)" python3 - <<'PY'
import json, os, sqlite3
from pathlib import Path
db = Path(os.environ["CURSOR_STATE_DB"])
key = os.environ["CURSOR_APPUSER_KEY"]
con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
row = con.execute("SELECT value FROM ItemTable WHERE key=?", (key,)).fetchone()
con.close()
if not row:
    print("applicationUser blob missing")
    raise SystemExit(1)
data = json.loads(row[0])
ai = data.get("aiSettings") or {}
models = ai.get("userAddedModels") or []
print(f"DB:            {db}")
print(f"useOpenAIKey:  {data.get('useOpenAIKey')}")
print(f"openAIBaseUrl: {data.get('openAIBaseUrl')}")
print(f"userAddedModels ({len(models)}):")
for m in models:
    print(f"  - {m}")
prefixed = [m for m in models if m.startswith(("claude/", "chatgpt/", "grok/", "inja/"))]
print(f"gateway-prefixed among them: {len(prefixed)}")
PY
  if _cursor_is_running; then
    echo "(Cursor appears to be running — quit fully before cursor-apply)" >&2
  fi
}

# Write openAIBaseUrl + merge gateway models into userAddedModels / modelOverrideEnabled.
# Does NOT write the OpenAI API key (Electron safeStorage).
cursor-apply() {
  local db base key force=0
  db="$(_cursor_state_db)"
  base="$(_cursor_base_url)"
  key="$(_cursor_key)"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      -f|--force) force=1; shift ;;
      -h|--help)
        cat <<EOF
usage: cursor-apply [--force]

  Fully quit Cursor first (or pass --force at your own risk).
  Merges gateway model names into Cursor's userAddedModels,
  enables useOpenAIKey, sets openAIBaseUrl → $base

  Still paste OpenAI API Key once in Settings (encrypted storage):
    $key
EOF
        return 0
        ;;
      *)
        echo "unknown arg: $1 (try --force)" >&2
        return 1
        ;;
    esac
  done

  if [[ ! -f "$db" ]]; then
    echo "Cursor state DB not found: $db" >&2
    echo "Open Cursor once so it creates the DB, then quit and re-run." >&2
    return 1
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 required for cursor-apply" >&2
    return 1
  fi
  if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "sqlite3 required for cursor-apply" >&2
    return 1
  fi

  if _cursor_is_running && [[ "$force" -ne 1 ]]; then
    echo "Cursor is running. Fully quit Cursor (Cmd+Q), then re-run cursor-apply." >&2
    echo "Or: cursor-apply --force   (may be overwritten when Cursor exits)" >&2
    return 1
  fi

  mkdir -p "$(_cursor_backup_dir)"
  local models_file bak stamp
  models_file="$(mktemp "${TMPDIR:-/tmp}/cursor-gw-models.XXXXXX")"
  _cursor_gateway_models >"$models_file"
  stamp="$(date +%Y%m%d%H%M%S)"
  bak="$(_cursor_backup_dir)/applicationUser.${stamp}.json"

  echo "cursor-apply → $db" >&2
  echo "  base URL: $base" >&2
  echo "  models:   $models_file" >&2

  local rc=0
  CURSOR_STATE_DB="$db" \
    CURSOR_APPUSER_KEY="$(_cursor_appuser_key)" \
    CURSOR_BASE_URL="$base" \
    CURSOR_MODELS_FILE="$models_file" \
    CURSOR_BACKUP_FILE="$bak" \
    python3 - <<'PY' || rc=$?
import json, os, sqlite3, sys
from pathlib import Path

db = Path(os.environ["CURSOR_STATE_DB"])
app_key = os.environ["CURSOR_APPUSER_KEY"]
base = os.environ["CURSOR_BASE_URL"]
models_file = Path(os.environ["CURSOR_MODELS_FILE"])
backup = Path(os.environ["CURSOR_BACKUP_FILE"])

wanted = [ln.strip() for ln in models_file.read_text().splitlines() if ln.strip() and not ln.strip().startswith("#")]

# Backup applicationUser JSON only (state.vscdb can be multi‑GB — do not full-copy).
con = sqlite3.connect(str(db))
cur = con.cursor()
row = cur.execute("SELECT value FROM ItemTable WHERE key=?", (app_key,)).fetchone()
if not row:
    print("applicationUser key missing in state.vscdb — open Cursor once, quit, retry", file=sys.stderr)
    con.close()
    sys.exit(2)

raw = row[0]
backup.write_text(raw if isinstance(raw, str) else raw.decode("utf-8", "replace"))
print(f"  applicationUser backup → {backup}", file=sys.stderr)

data = json.loads(raw)
ai = data.setdefault("aiSettings", {})
existing = list(ai.get("userAddedModels") or [])
enabled = list(ai.get("modelOverrideEnabled") or [])
disabled = list(ai.get("modelOverrideDisabled") or [])

# preserve order: existing first, then new gateway names
seen = set(existing)
merged = list(existing)
added = []
for m in wanted:
    if m not in seen:
        merged.append(m)
        seen.add(m)
        added.append(m)

ai["userAddedModels"] = merged

# enable in picker
en_set = set(enabled)
for m in wanted:
    if m not in en_set:
        enabled.append(m)
        en_set.add(m)
ai["modelOverrideEnabled"] = enabled
ai["modelOverrideDisabled"] = [m for m in disabled if m not in set(wanted)]

data["useOpenAIKey"] = True
data["openAIBaseUrl"] = base

new_raw = json.dumps(data, separators=(",", ":"), ensure_ascii=False)
cur.execute("UPDATE ItemTable SET value=? WHERE key=?", (new_raw, app_key))
if cur.rowcount == 0:
    cur.execute("INSERT INTO ItemTable (key, value) VALUES (?, ?)", (app_key, new_raw))
con.commit()
con.close()

print(f"  openAIBaseUrl = {base}", file=sys.stderr)
print(f"  userAddedModels now {len(merged)} (added {len(added)} new)", file=sys.stderr)
for m in added:
    print(f"    + {m}", file=sys.stderr)
if not added:
    print("  (all gateway models already present)", file=sys.stderr)
print("OK", file=sys.stderr)
PY
  rm -f "$models_file"
  if [[ "$rc" -ne 0 ]]; then
    echo "cursor-apply failed (exit $rc)" >&2
    return "$rc"
  fi
  cat <<EOF >&2

Done. Next:
  1) Open Cursor
  2) Settings → Models → OpenAI API Key = $key  (once; if already set, skip)
  3) Confirm Override Base URL is $base
  4) Pick claude/fable-5 (gateway) or Claude Fable 5 (Cursor)

Rollback: cursor-rollback
Status:   cursor-status
EOF
  return 0
}

cursor-rollback() {
  local db bakdir latest
  db="$(_cursor_state_db)"
  bakdir="$(_cursor_backup_dir)"
  if [[ ! -d "$bakdir" ]]; then
    echo "no backups in $bakdir" >&2
    return 1
  fi
  # shellcheck disable=SC2012
  latest="$(ls -1t "$bakdir"/applicationUser.*.json 2>/dev/null | head -1 || true)"
  if [[ -z "$latest" ]]; then
    echo "no applicationUser.*.json backups in $bakdir" >&2
    return 1
  fi
  if _cursor_is_running && [[ "${1:-}" != "--force" ]]; then
    echo "Quit Cursor fully, then: cursor-rollback" >&2
    return 1
  fi
  if ! command -v python3 >/dev/null 2>&1 || ! command -v sqlite3 >/dev/null 2>&1; then
    echo "python3 and sqlite3 required" >&2
    return 1
  fi
  echo "restoring $latest → $db" >&2
  CURSOR_STATE_DB="$db" CURSOR_APPUSER_KEY="$(_cursor_appuser_key)" CURSOR_RESTORE_FILE="$latest" python3 - <<'PY'
import os, sqlite3, sys
from pathlib import Path
db = Path(os.environ["CURSOR_STATE_DB"])
key = os.environ["CURSOR_APPUSER_KEY"]
src = Path(os.environ["CURSOR_RESTORE_FILE"])
raw = src.read_text()
con = sqlite3.connect(str(db))
cur = con.cursor()
cur.execute("UPDATE ItemTable SET value=? WHERE key=?", (raw, key))
if cur.rowcount == 0:
    cur.execute("INSERT INTO ItemTable (key, value) VALUES (?, ?)", (key, raw))
con.commit()
con.close()
print("restored applicationUser JSON — reopen Cursor", file=sys.stderr)
PY
}

cursor-write-cheatsheet() {
  local root out base key
  root="$(_inja_gateway_root)"
  out="${1:-$root/examples/cursor/SETUP.md}"
  mkdir -p "$(dirname "$out")"
  base="$(_cursor_base_url)"
  key="$(_cursor_key)"
  cat >"$out" <<EOF
# Cursor → Inja LLM Gateway

## Automated

\`\`\`bash
cc-gateway-up
# fully quit Cursor
cursor-apply
# reopen Cursor; paste API key once if needed: $key
\`\`\`

## Manual Settings

| Field | Value |
|-------|-------|
| OpenAI API Key | \`$key\` |
| Override OpenAI Base URL | \`$base\` |

## Custom models

\`\`\`
$(_cursor_gateway_models)
\`\`\`
EOF
  echo "wrote $out" >&2
}

cursor-verify() {
  local base key body
  base="$(_cursor_base_url)"
  key="$(_cursor_key)"
  echo "GET $base/models (look for claude/fable-5, inja/…)" >&2
  body="$(curl -skS "$base/models" \
    -H "Authorization: Bearer $key" \
    -H "Content-Type: application/json" 2>/dev/null || true)"
  if [[ -z "$body" ]]; then
    echo "empty response — is the gateway up? (cc-gateway-up)" >&2
    return 1
  fi
  printf '%s\n' "$body" | head -c 2000
  echo
  echo "── prefixed gateway ids ──" >&2
  printf '%s\n' "$body" | _cursor_filter_prefixed || true
}

cursor-print() { cursor-setup "$@"; }
cursor-help()  { cursor-setup "$@"; }
cursor-list-models() { cursor-models "$@"; }
cursor-install() { cursor-apply "$@"; }
cursor-enable() { cursor-apply "$@"; }
