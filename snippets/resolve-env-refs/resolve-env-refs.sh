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
_RENV_CACHE_FILE=""    # path to the encrypted on-RAM cache file we own

# Max age of a RAM cache file before it is discarded and re-fetched (seconds).
_RENV_CACHE_MAX_AGE=28800  # 8 hours

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

# ── BW item cache (RAM-backed, AES-256 encrypted) ─────────────────────────────
#
# Cache file lives in /dev/shm (tmpfs RAM) so it survives across sourcing the
# same .env in multiple shells without a second bw list items round-trip.
#
# Naming:  /dev/shm/renv-<first16 of SHA-256(BW_SESSION)>.enc
#   - Unique per session token → no collision between concurrent logins
#   - Shared across shells with the *same* BW_SESSION → only one slow fetch
#
# Security notes:
#   - Encrypted with AES-256-CBC + PBKDF2 (openssl enc); key derived from
#     SHA-256(BW_SESSION) so the file is useless without the session token.
#   - chmod 600: inaccessible to other users on a shared machine.
#   - /dev/shm is RAM-only on Linux; never written to a physical disk.
#   - macOS has no /dev/shm — falls back to /tmp (disk-backed); note in README.
#   - Root and SIGKILL bypass all of this (inherent bash/shell limitation).
#   - Files older than _RENV_CACHE_MAX_AGE are discarded and re-fetched.

_renv_cache_dir() {
  if [[ -d /dev/shm && -w /dev/shm ]]; then
    printf '/dev/shm'
  else
    printf '/tmp'
  fi
}

_renv_cache_file() {
  local session="$1"
  local id
  id=$(printf '%s' "$session" | sha256sum 2>/dev/null | cut -c1-16) \
    || id=$(printf '%s' "$session" | openssl dgst -sha256 2>/dev/null | awk '{print substr($NF,1,16)}')
  printf '%s/renv-%s.enc' "$(_renv_cache_dir)" "$id"
}

_renv_cache_key() {
  local session="$1"
  # Full SHA-256 hex string used as passphrase; openssl PBKDF2 derives the key.
  printf '%s' "$session" | sha256sum 2>/dev/null | awk '{print $1}' \
    || printf '%s' "$session" | openssl dgst -sha256 2>/dev/null | awk '{print $NF}'
}

_renv_cache_stale() {
  local file="$1"
  local now age mtime
  now=$(date +%s)
  # stat is not portable; try both Linux and macOS forms.
  mtime=$(stat -c '%Y' "$file" 2>/dev/null) \
    || mtime=$(stat -f '%m' "$file" 2>/dev/null) \
    || { printf 'resolve-env-refs: cannot stat cache file\n' >&2; return 0; }
  age=$(( now - mtime ))
  (( age > _RENV_CACHE_MAX_AGE ))
}

_renv_bw_items() {
  if [[ -z "$_RENV_BW_ITEMS" ]]; then
    local session
    session=$(_renv_bw_session) || return 1

    local cache_file cache_key
    cache_file=$(_renv_cache_file "$session")
    cache_key=$(_renv_cache_key  "$session")

    # ── Try the RAM cache first ───────────────────────────────────────────────
    if [[ -f "$cache_file" ]]; then
      if _renv_cache_stale "$cache_file"; then
        printf 'resolve-env-refs: RAM cache expired, re-fetching from Bitwarden...\n' >&2
        rm -f "$cache_file"
      else
        local decrypted
        decrypted=$(openssl enc -d -aes-256-cbc -pbkdf2 \
          -pass "pass:${cache_key}" -in "$cache_file" 2>/dev/null) || {
          printf 'resolve-env-refs: cache decrypt failed (stale session?), re-fetching...\n' >&2
          rm -f "$cache_file"
          decrypted=""
        }
        if [[ -n "$decrypted" ]]; then
          _RENV_BW_ITEMS="$decrypted"
          _RENV_CACHE_FILE="$cache_file"
          printf '%s' "$_RENV_BW_ITEMS"
          return 0
        fi
      fi
    fi

    # ── Cache miss: fetch from Bitwarden ─────────────────────────────────────
    _renv_bw_check || return 1
    printf 'resolve-env-refs: fetching items from Bitwarden (this happens once)...\n' >&2

    _RENV_BW_ITEMS=$(bw list items --session "$session" 2>/dev/null) || {
      printf 'resolve-env-refs: bw list items failed\n' >&2
      return 1
    }

    if [[ "$_RENV_BW_ITEMS" == "[]" || -z "$_RENV_BW_ITEMS" ]]; then
      printf 'resolve-env-refs: bw returned no items — try: bw sync\n' >&2
      _RENV_BW_ITEMS=""
      return 1
    fi

    # ── Encrypt and write to RAM ──────────────────────────────────────────────
    printf '%s' "$_RENV_BW_ITEMS" \
      | openssl enc -aes-256-cbc -pbkdf2 \
          -pass "pass:${cache_key}" -out "$cache_file" 2>/dev/null \
      && chmod 600 "$cache_file" \
      && _RENV_CACHE_FILE="$cache_file" \
      || printf 'resolve-env-refs: warning: could not write RAM cache (%s)\n' "$cache_file" >&2
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
  if [[ -n "$_RENV_CACHE_FILE" && -f "$_RENV_CACHE_FILE" ]]; then
    rm -f "$_RENV_CACHE_FILE"
  fi
  _RENV_CACHE_FILE=""
}
