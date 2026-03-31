#!/usr/bin/env bash
# resolve-env-refs.sh — Resolve bw:// and vault:// secret refs in .env files.
#
# ── Reference formats ─────────────────────────────────────────────────────────
#
#   bw://item-name              → Bitwarden password field (default)
#   bw://item-name/password     → Bitwarden password field
#   bw://item-name/username     → Bitwarden username field
#   bw://item-name/note         → Bitwarden notes field
#   bw://item-name/field:fname  → Bitwarden custom field "fname"
#   vault://secret/path#field   → Vault KV v2 field
#
# ── Usage ─────────────────────────────────────────────────────────────────────
#
#   Pattern 1 — direnv (.envrc):
#     source ~/.config/resolve-env-refs/resolve-env-refs.sh
#     source <(resolve_env_file .env)
#
#   Pattern 2 — self-loading .env (first line of your .env):
#     source ~/.config/resolve-env-refs/resolve-env-refs.sh \
#       && source <(resolve_env_file "${BASH_SOURCE[0]:-$0}") \
#       && return 0 2>/dev/null; true
#
#     DATABASE_URL=bw://prod-db/password
#     STRIPE_KEY=vault://secret/stripe#api_key
#
# ── Cleanup ───────────────────────────────────────────────────────────────────
#
#   Secrets are tracked and unset automatically on shell EXIT.
#   Manual cleanup: unload_env
#
# ── Prerequisites ─────────────────────────────────────────────────────────────
#
#   bw + jq   — for bw:// references
#   vault     — for vault:// references (VAULT_ADDR + VAULT_TOKEN must be set)
#

# ── Internal state ────────────────────────────────────────────────────────────

_RENV_BW_SESSION=""    # cached bw session token
_RENV_BW_ITEMS=""      # cached JSON from: bw list items
_RENV_VARS=()          # names of vars we exported (for cleanup)

# ── BW session ────────────────────────────────────────────────────────────────

_renv_bw_check() {
  # Verify bw is logged in and the vault is unlocked.
  # Returns 0 if ready, 1 with a helpful message if not.
  if ! command -v bw >/dev/null 2>&1; then
    printf 'resolve-env-refs: bw CLI not found\n' >&2
    return 1
  fi

  local status
  status=$(bw status 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4)

  case "$status" in
    unlocked)
      return 0
      ;;
    locked)
      printf 'resolve-env-refs: Bitwarden vault is locked.\n' >&2
      printf '  Run:  export BW_SESSION=$(bw unlock --raw)\n' >&2
      printf '  Then source your .env again.\n' >&2
      return 1
      ;;
    unauthenticated)
      printf 'resolve-env-refs: Not logged in to Bitwarden.\n' >&2
      printf '  Run:  bw login\n' >&2
      printf '  Then: export BW_SESSION=$(bw unlock --raw)\n' >&2
      return 1
      ;;
    *)
      printf 'resolve-env-refs: unexpected bw status: %s\n' "${status:-unknown}" >&2
      printf '  Run:  export BW_SESSION=$(bw unlock --raw)\n' >&2
      return 1
      ;;
  esac
}

_renv_bw_session() {
  if [[ -n "${BW_SESSION:-}" ]]; then
    printf '%s' "$BW_SESSION"
    return 0
  fi

  if [[ -n "$_RENV_BW_SESSION" ]]; then
    printf '%s' "$_RENV_BW_SESSION"
    return 0
  fi

  # No session available — guide the user rather than prompting interactively.
  printf 'resolve-env-refs: BW_SESSION is not set.\n' >&2
  printf '  Run:  export BW_SESSION=$(bw unlock --raw)\n' >&2
  printf '  If items are missing, also run:  bw sync\n' >&2
  return 1
}

# ── BW item cache ─────────────────────────────────────────────────────────────

_renv_bw_items() {
  if [[ -z "$_RENV_BW_ITEMS" ]]; then
    local session
    session=$(_renv_bw_session) || return 1

    # Sanity-check login/lock state before the (slow) list call.
    _renv_bw_check || return 1

    _RENV_BW_ITEMS=$(bw list items --session "$session" 2>/dev/null) || {
      printf 'resolve-env-refs: bw list items failed\n' >&2
      return 1
    }

    if [[ "$_RENV_BW_ITEMS" == "[]" || -z "$_RENV_BW_ITEMS" ]]; then
      printf 'resolve-env-refs: bw returned no items — try: bw sync\n' >&2
      _RENV_BW_ITEMS=""
      return 1
    fi
  fi
  printf '%s' "$_RENV_BW_ITEMS"
}

# ── BW value resolver ─────────────────────────────────────────────────────────

_renv_bw_get() {
  local name="$1" field="${2:-password}"

  local items item
  items=$(_renv_bw_items) || return 1

  item=$(printf '%s' "$items" \
    | jq -r --arg n "$name" 'first(.[] | select(.name == $n))') 2>/dev/null

  if [[ -z "$item" || "$item" == "null" ]]; then
    printf 'resolve-env-refs: bw item not found: %s\n' "$name" >&2
    return 1
  fi

  case "$field" in
    password) printf '%s' "$item" | jq -r '.login.password // empty' ;;
    username) printf '%s' "$item" | jq -r '.login.username // empty' ;;
    note)     printf '%s' "$item" | jq -r '.notes // empty' ;;
    field:*)
      local fname="${field#field:}"
      printf '%s' "$item" \
        | jq -r --arg f "$fname" '.fields[]? | select(.name == $f) | .value // empty'
      ;;
    *)
      printf 'resolve-env-refs: unknown bw field: %s\n' "$field" >&2
      return 1
      ;;
  esac
}

# ── Vault value resolver ──────────────────────────────────────────────────────

_renv_vault_get() {
  local path="$1" field="$2"

  if [[ -z "$field" ]]; then
    printf 'resolve-env-refs: vault:// requires a #field specifier (vault://%s#field)\n' "$path" >&2
    return 1
  fi

  vault kv get -field="$field" "$path" 2>/dev/null || {
    printf 'resolve-env-refs: vault kv get failed: %s#%s\n' "$path" "$field" >&2
    return 1
  }
}

# ── Ref dispatcher ────────────────────────────────────────────────────────────

_renv_resolve() {
  local ref="$1"
  case "$ref" in
    bw://*)
      local rest="${ref#bw://}"
      local name field="password"
      if [[ "$rest" == */* ]]; then
        name="${rest%%/*}"
        field="${rest#*/}"
      else
        name="$rest"
      fi
      _renv_bw_get "$name" "$field"
      ;;
    vault://*)
      local rest="${ref#vault://}"
      local path field=""
      if [[ "$rest" == *#* ]]; then
        path="${rest%%#*}"
        field="${rest#*#}"
      else
        path="$rest"
      fi
      _renv_vault_get "$path" "$field"
      ;;
    *)
      printf '%s' "$ref"
      ;;
  esac
}

# ── Main public function ──────────────────────────────────────────────────────
#
# resolve_env_file <file>
#
#   Reads <file>, resolves bw:// and vault:// values, and emits:
#     export KEY='value'
#   for each KEY=VALUE line.  Source the output:
#     source <(resolve_env_file .env)
#

resolve_env_file() {
  local file="${1:-.env}"

  if [[ ! -f "$file" ]]; then
    printf 'resolve-env-refs: file not found: %s\n' "$file" >&2
    return 1
  fi

  local line key raw resolved

  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip blanks and comments.
    [[ "$line" =~ ^[[:space:]]*$ ]]  && continue
    [[ "$line" =~ ^[[:space:]]*#  ]] && continue
    # Skip shell directives (e.g. the self-loading header, export statements
    # that aren't KEY=VALUE, source lines, etc.).
    [[ "$line" =~ ^[[:space:]]*(source|export[[:space:]]+[^=]|declare|typeset) ]] && continue

    # Match: [export] KEY=VALUE  (optional surrounding quotes stripped below)
    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      key="${BASH_REMATCH[2]}"
      raw="${BASH_REMATCH[3]}"

      # Strip one layer of surrounding single or double quotes.
      if [[ "$raw" =~ ^\"(.*)\"$ ]]; then raw="${BASH_REMATCH[1]}"; fi
      if [[ "$raw" =~ ^\'(.*)\'$ ]]; then raw="${BASH_REMATCH[1]}"; fi

      if [[ "$raw" == bw://* || "$raw" == vault://* ]]; then
        resolved=$(_renv_resolve "$raw") || return 1
      else
        resolved="$raw"
      fi

      # Emit as a safely shell-quoted export (%q handles all special chars).
      printf 'export %s=%q\n' "$key" "$resolved"
      _RENV_VARS+=("$key")
    fi
  done <"$file"

  # Register cleanup trap (chain any existing EXIT handler).
  local existing_trap
  existing_trap=$(trap -p EXIT 2>/dev/null | sed "s/trap -- '//;s/' EXIT//")
  if [[ -n "$existing_trap" && "$existing_trap" != *unload_env* ]]; then
    # shellcheck disable=SC2064
    trap "${existing_trap}; unload_env" EXIT
  else
    trap unload_env EXIT
  fi
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

unload_env() {
  local v
  for v in "${_RENV_VARS[@]+"${_RENV_VARS[@]}"}"; do
    unset "$v" 2>/dev/null || true
  done
  _RENV_VARS=()
  _RENV_BW_ITEMS=""
  _RENV_BW_SESSION=""
}
