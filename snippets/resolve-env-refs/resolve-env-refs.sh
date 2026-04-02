#!/usr/bin/env bash
# resolve-env-refs.sh — Resolve bw:// and vault:// secret refs in .env files.
#
# ── Reference formats ─────────────────────────────────────────────────────────
#
#   bw://folder/item-name              → Bitwarden password field (default)
#   bw://folder/item-name/password     → Bitwarden password field
#   bw://folder/item-name/username     → Bitwarden username field
#   bw://folder/item-name/note         → Bitwarden notes field
#   bw://folder/item-name/field:fname  → Bitwarden custom field "fname"
#   vault://secret/path#field          → Vault KV v2 field
#
#   The folder segment is REQUIRED for bw:// references. It scopes the Bitwarden
#   fetch to only items in that folder, minimising data pulled into RAM.
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
#     DATABASE_URL=bw://production/prod-db/password
#     STRIPE_KEY=vault://secret/stripe#api_key
#
# ── Authentication ────────────────────────────────────────────────────────────
#
#   On first use the script prompts for your Bitwarden master password, or reads
#   it from the RENV_BW_PASSWORD environment variable for non-interactive use:
#
#     export RENV_BW_PASSWORD=<master-password>
#     source <(resolve_env_file .env)
#
#   The master password is used for two purposes:
#     1. Unlock the Bitwarden vault via: bw unlock --passwordenv
#     2. Derive the AES-256 encryption key for the on-RAM folder cache
#
#   After all folder data is fetched the BW_SESSION token is cleared from memory
#   and the environment.  The encrypted cache survives across shells so future
#   sourcing only requires the master password to decrypt — no Bitwarden network
#   call needed.
#
# ── Cleanup ───────────────────────────────────────────────────────────────────
#
#   Secrets are tracked and unset automatically on shell EXIT.
#   Manual cleanup:              unload_env
#   Remove on-disk cache files:  renv_clear_cache
#
# ── Prerequisites ─────────────────────────────────────────────────────────────
#
#   bw + jq   — for bw:// references (must be logged in: bw login)
#   openssl   — for AES-256 cache encryption
#   vault     — for vault:// references (VAULT_ADDR + VAULT_TOKEN must be set)
#

# ── Internal state ────────────────────────────────────────────────────────────

_RENV_BW_SESSION=""          # ephemeral BW session token; cleared after folder fetch
_RENV_BW_PW_PLAIN=""         # plaintext password held briefly between init & unlock
_RENV_CACHE_KEY=""           # SHA-256(master_password); survives for this shell session
_RENV_BW_ACCT_TAG=""         # 8-char fingerprint of BW account (email+server); prevents cross-account cache collisions
_RENV_BW_ITEMS="[]"          # merged JSON array of items from all fetched folders
_RENV_BW_FETCHED_FOLDERS=()  # folder names already loaded into _RENV_BW_ITEMS
_RENV_BW_FOLDER_IDS=()       # BW folder UUIDs — parallel to _RENV_BW_FETCHED_FOLDERS
_RENV_VARS=()                # names of vars we exported (for cleanup)
_RENV_CACHE_FILES=()         # disk cache files created by this session (for purge)

# Max age of a per-folder RAM cache file before it is discarded (seconds).
_RENV_CACHE_MAX_AGE=28800  # 8 hours

# ── SHA-256 helper ────────────────────────────────────────────────────────────

_renv_sha256() {
  # Return the SHA-256 hex digest of stdin string $1.
  printf '%s' "$1" | sha256sum 2>/dev/null | awk '{print $1}' \
    || printf '%s' "$1" | openssl dgst -sha256 2>/dev/null | awk '{print $NF}'
}

# ── Array helper ──────────────────────────────────────────────────────────────

_renv_in_array() {
  local val="$1"; shift
  local item
  for item in "$@"; do
    [[ "$item" == "$val" ]] && return 0
  done
  return 1
}

# ── BW account fingerprint ────────────────────────────────────────────────────

_renv_bw_acct_tag() {
  # Return an 8-char fingerprint of the active BW account (email + serverUrl).
  # Uses `bw status` which works even when the vault is locked.
  # Cached in _RENV_BW_ACCT_TAG to avoid repeated bw status calls.
  # Falls back to "unknown" if bw is unavailable — prevents errors but
  # means cache is not account-scoped in that case (acceptable degradation).
  if [[ -n "$_RENV_BW_ACCT_TAG" ]]; then
    printf '%s' "$_RENV_BW_ACCT_TAG"
    return 0
  fi

  local status_json email server identity
  status_json=$(bw status 2>/dev/null) || { _RENV_BW_ACCT_TAG="unknown"; printf 'unknown'; return 0; }
  email=$(printf '%s' "$status_json"  | jq -r '.userEmail  // empty' 2>/dev/null)
  server=$(printf '%s' "$status_json" | jq -r '.serverUrl  // empty' 2>/dev/null)
  identity="${email}:${server}"

  if [[ -z "$email" && -z "$server" ]]; then
    _RENV_BW_ACCT_TAG="unknown"
  else
    _RENV_BW_ACCT_TAG=$(_renv_sha256 "$identity" | cut -c1-8)
  fi
  printf '%s' "$_RENV_BW_ACCT_TAG"
}

# ── Cache key: ensure master-password hash is available ──────────────────────

_renv_ensure_cache_key() {
  # Sets _RENV_CACHE_KEY from the master password.
  # Also stores the plaintext briefly in _RENV_BW_PW_PLAIN for vault unlock.
  if [[ -n "$_RENV_CACHE_KEY" ]]; then return 0; fi

  local password
  if [[ -n "${RENV_BW_PASSWORD:-}" ]]; then
    password="$RENV_BW_PASSWORD"
  else
    printf 'resolve-env-refs: Bitwarden master password required.\n' >&2
    printf '  (Set RENV_BW_PASSWORD to skip this prompt.)\n' >&2
    printf '  Password: ' >/dev/tty
    IFS= read -rs password </dev/tty
    printf '\n' >/dev/tty
  fi

  if [[ -z "$password" ]]; then
    printf 'resolve-env-refs: password cannot be empty\n' >&2
    return 1
  fi

  _RENV_CACHE_KEY=$(_renv_sha256 "$password")
  _RENV_BW_PW_PLAIN="$password"  # held briefly; cleared by _renv_ensure_bw_session
  password=""
}

# ── BW session: unlock vault (only when a live fetch is needed) ───────────────

_renv_ensure_bw_session() {
  # Sets _RENV_BW_SESSION. Assumes _renv_ensure_cache_key was already called.

  # Fast paths: already have a session
  if [[ -n "$_RENV_BW_SESSION" ]]; then
    _RENV_BW_PW_PLAIN=""
    return 0
  fi
  if [[ -n "${BW_SESSION:-}" ]]; then
    _RENV_BW_SESSION="$BW_SESSION"
    _RENV_BW_PW_PLAIN=""
    return 0
  fi

  if ! command -v bw >/dev/null 2>&1; then
    printf 'resolve-env-refs: bw CLI not found\n' >&2
    _RENV_BW_PW_PLAIN=""
    return 1
  fi

  local bw_status
  bw_status=$(bw status 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4)

  case "$bw_status" in
    unauthenticated)
      printf 'resolve-env-refs: not logged in to Bitwarden.\n' >&2
      printf '  Run:  bw login\n' >&2
      _RENV_BW_PW_PLAIN=""
      return 1
      ;;
    locked)
      local pw="${_RENV_BW_PW_PLAIN:-}"
      if [[ -z "$pw" && -n "${RENV_BW_PASSWORD:-}" ]]; then
        pw="$RENV_BW_PASSWORD"
      fi
      if [[ -z "$pw" ]]; then
        printf 'resolve-env-refs: need master password to unlock vault.\n' >&2
        printf '  Password: ' >/dev/tty
        IFS= read -rs pw </dev/tty
        printf '\n' >/dev/tty
      fi

      printf 'resolve-env-refs: unlocking Bitwarden vault...\n' >&2
      local _pv="_RENV_UNLOCK_$$"
      declare -x "$_pv"="$pw"
      _RENV_BW_SESSION=$(bw unlock --passwordenv "$_pv" --raw 2>/dev/null)
      local rc=$?
      unset "$_pv" 2>/dev/null || true
      pw=""
      _RENV_BW_PW_PLAIN=""

      if [[ $rc -ne 0 || -z "$_RENV_BW_SESSION" ]]; then
        printf 'resolve-env-refs: bw unlock failed — wrong password?\n' >&2
        _RENV_BW_SESSION=""
        _RENV_CACHE_KEY=""  # reset so next attempt re-derives from correct password
        return 1
      fi
      ;;
    unlocked)
      # Vault is unlocked but BW_SESSION is not exported — user needs to export it.
      printf 'resolve-env-refs: vault is unlocked but BW_SESSION is not set.\n' >&2
      printf '  Run:  export BW_SESSION=$(bw unlock --raw)\n' >&2
      _RENV_BW_PW_PLAIN=""
      return 1
      ;;
    *)
      printf 'resolve-env-refs: unexpected bw status: %s\n' "${bw_status:-unknown}" >&2
      _RENV_BW_PW_PLAIN=""
      return 1
      ;;
  esac
}

# ── BW session teardown ───────────────────────────────────────────────────────

_renv_bw_clear_session() {
  # Discard the ephemeral BW session token from memory and the environment.
  # The folder item caches (encrypted on /dev/shm) are retained — future shells
  # can decrypt them with the master password without touching Bitwarden.
  if [[ -n "$_RENV_BW_SESSION" ]]; then
    _RENV_BW_SESSION=""
    unset BW_SESSION 2>/dev/null || true
    printf 'resolve-env-refs: BW session cleared — vault data is now cache-only.\n' >&2
  fi
}

# ── BW folder-item cache (RAM-backed, AES-256 encrypted) ──────────────────────
#
# One cache file per Bitwarden folder; lives in /dev/shm (tmpfs RAM) on Linux.
#
# Naming:  /dev/shm/renv-<first16 of SHA-256(uid:folder_name)>.enc
#   - Scoped to the OS user (uid prefix prevents cross-user collisions).
#   - One file per folder — only relevant items stored, not the whole vault.
#   - Shared across shells for the same user/folder — one slow fetch per TTL.
#
# Encryption key:  SHA-256(master_password)
#   - openssl PBKDF2 further stretches this with a random salt stored in the
#     file header, so the on-disk ciphertext is useless without the master pw.
#   - Survives across shells: new shell re-prompts for the password, derives
#     the same key, and decrypts without contacting Bitwarden.
#   - chmod 600: inaccessible to other users on a shared machine.
#   - /dev/shm is RAM-only on Linux; macOS falls back to /tmp (disk-backed).

_renv_cache_dir() {
  if [[ -d /dev/shm && -w /dev/shm ]]; then
    printf '/dev/shm'
  else
    printf '/tmp'
  fi
}

_renv_cache_file() {
  # Return cache file path keyed by uid:account_fingerprint:folder_name.
  # Including the account fingerprint prevents cross-account cache collisions
  # when the same OS user switches between Bitwarden accounts.
  local folder="$1"
  local uid_str="${UID:-$(id -u 2>/dev/null || echo 0)}"
  local acct_tag
  acct_tag=$(_renv_bw_acct_tag)
  local hash
  hash=$(_renv_sha256 "${uid_str}:${acct_tag}:${folder}")
  printf '%s/renv-%s.enc' "$(_renv_cache_dir)" "${hash:0:16}"
}

_renv_cache_stale() {
  local file="$1"
  local now mtime age
  now=$(date +%s)
  mtime=$(stat -c '%Y' "$file" 2>/dev/null) \
    || mtime=$(stat -f '%m' "$file" 2>/dev/null) \
    || { printf 'resolve-env-refs: cannot stat cache file\n' >&2; return 0; }
  age=$(( now - mtime ))
  (( age > _RENV_CACHE_MAX_AGE ))
}

# ── Folder ID lookup (from in-memory parallel arrays) ─────────────────────────

_renv_folder_id_lookup() {
  local folder="$1"
  local i
  for (( i=0; i<${#_RENV_BW_FETCHED_FOLDERS[@]}; i++ )); do
    if [[ "${_RENV_BW_FETCHED_FOLDERS[$i]}" == "$folder" ]]; then
      printf '%s' "${_RENV_BW_FOLDER_IDS[$i]}"
      return 0
    fi
  done
  return 1
}

# ── Merge JSON item arrays ────────────────────────────────────────────────────

_renv_merge_items() {
  local new_items="$1"
  if [[ "$_RENV_BW_ITEMS" == "[]" || -z "$_RENV_BW_ITEMS" ]]; then
    _RENV_BW_ITEMS="$new_items"
  else
    _RENV_BW_ITEMS=$(printf '%s\n%s' "$_RENV_BW_ITEMS" "$new_items" \
      | jq -s 'add // []' 2>/dev/null)
  fi
}

# ── Per-folder item loader ────────────────────────────────────────────────────

_renv_bw_folder_items() {
  local folder="$1"

  # Already loaded in this shell session?
  if _renv_in_array "$folder" "${_RENV_BW_FETCHED_FOLDERS[@]+"${_RENV_BW_FETCHED_FOLDERS[@]}"}"; then
    return 0
  fi

  # Pre-populate _RENV_BW_ACCT_TAG in the parent shell so command substitutions
  # (subshells) inherit the cached value and avoid repeated `bw status` calls.
  _renv_bw_acct_tag > /dev/null

  # Need the cache key (master password hash) to check the disk cache.
  _renv_ensure_cache_key || return 1

  local cache_file
  cache_file=$(_renv_cache_file "$folder")

  # ── Try on-disk cache first ───────────────────────────────────────────────
  if [[ -f "$cache_file" ]]; then
    if _renv_cache_stale "$cache_file"; then
      printf 'resolve-env-refs: cache expired for folder "%s", re-fetching...\n' "$folder" >&2
      rm -f "$cache_file"
    else
      local decrypted
      decrypted=$(openssl enc -d -aes-256-cbc -pbkdf2 \
        -pass "pass:${_RENV_CACHE_KEY}" -in "$cache_file" 2>/dev/null) || {
        printf 'resolve-env-refs: cache decrypt failed for "%s" (wrong password?), re-fetching...\n' "$folder" >&2
        rm -f "$cache_file"
        decrypted=""
      }
      if [[ -n "$decrypted" ]]; then
        _renv_merge_items "$decrypted"
        local fid
        fid=$(printf '%s' "$decrypted" | jq -r 'first(.[]).folderId // empty' 2>/dev/null)
        _RENV_BW_FETCHED_FOLDERS+=("$folder")
        _RENV_BW_FOLDER_IDS+=("${fid:-}")
        _RENV_BW_PW_PLAIN=""  # no longer needed
        return 0
      fi
    fi
  fi

  # ── Cache miss: fetch from Bitwarden ────────────────────────────────────────
  _renv_ensure_bw_session || return 1

  printf 'resolve-env-refs: fetching folder "%s" from Bitwarden...\n' "$folder" >&2

  # Resolve folder name → BW folder UUID (exact match after --search pre-filter)
  local folders_json folder_id
  folders_json=$(bw list folders --search "$folder" --session "$_RENV_BW_SESSION" 2>/dev/null) || {
    printf 'resolve-env-refs: bw list folders failed\n' >&2
    return 1
  }
  folder_id=$(printf '%s' "$folders_json" \
    | jq -r --arg n "$folder" 'first(.[] | select(.name == $n)) | .id // empty' 2>/dev/null)

  if [[ -z "$folder_id" ]]; then
    printf 'resolve-env-refs: Bitwarden folder not found: "%s"\n' "$folder" >&2
    printf '  Verify the folder name (case-sensitive). Check with:\n' >&2
    printf '    bw list folders | jq -r ".[].name"\n' >&2
    return 1
  fi

  # Fetch only items in this folder
  local items
  items=$(bw list items --folderid "$folder_id" --session "$_RENV_BW_SESSION" 2>/dev/null) || {
    printf 'resolve-env-refs: bw list items failed for folder "%s"\n' "$folder" >&2
    return 1
  }

  if [[ "$items" == "[]" || -z "$items" ]]; then
    printf 'resolve-env-refs: folder "%s" has no items (try: bw sync)\n' "$folder" >&2
    return 1
  fi

  # Merge into in-memory store
  _renv_merge_items "$items"
  _RENV_BW_FETCHED_FOLDERS+=("$folder")
  _RENV_BW_FOLDER_IDS+=("$folder_id")

  # Encrypt and write to RAM-backed cache (persists across shells)
  printf '%s' "$items" \
    | openssl enc -aes-256-cbc -pbkdf2 \
        -pass "pass:${_RENV_CACHE_KEY}" -out "$cache_file" 2>/dev/null \
    && chmod 600 "$cache_file" \
    && _RENV_CACHE_FILES+=("$cache_file") \
    || printf 'resolve-env-refs: warning: could not write disk cache for "%s"\n' "$folder" >&2

  return 0
}

# ── BW value resolver ─────────────────────────────────────────────────────────

_renv_bw_get() {
  local folder="$1" name="$2" field="${3:-password}"

  _renv_bw_folder_items "$folder" || return 1

  local folder_id
  folder_id=$(_renv_folder_id_lookup "$folder") || {
    printf 'resolve-env-refs: internal error: folder ID not cached for "%s"\n' "$folder" >&2
    return 1
  }

  local item
  item=$(printf '%s' "$_RENV_BW_ITEMS" \
    | jq -r --arg n "$name" --arg fid "$folder_id" \
      'first(.[] | select(.name == $n and .folderId == $fid))' 2>/dev/null)

  if [[ -z "$item" || "$item" == "null" ]]; then
    printf 'resolve-env-refs: item "%s" not found in folder "%s"\n' "$name" "$folder" >&2
    return 1
  fi

  case "$field" in
    password) printf '%s' "$item" | jq -r '.login.password // empty' ;;
    username) printf '%s' "$item" | jq -r '.login.username // empty' ;;
    note|notes) printf '%s' "$item" | jq -r '.notes // empty' ;;
    totp)     printf '%s' "$item" | jq -r '.login.totp // empty' ;;
    field:*)
      local fname="${field#field:}"
      printf '%s' "$item" \
        | jq -r --arg f "$fname" '.fields[]? | select(.name == $f) | .value // empty'
      ;;
    *)
      printf 'resolve-env-refs: unknown field "%s" (valid: password, username, note, totp, field:<name>)\n' "$field" >&2
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
      # New format requires folder: bw://folder/item-name[/field]
      if [[ "$rest" != */* ]]; then
        printf 'resolve-env-refs: invalid bw:// reference: %s\n' "$ref" >&2
        printf '  Format is now: bw://folder/item-name[/field]\n' >&2
        printf '  Example:       bw://production/db-password\n' >&2
        printf '  The folder segment is required to scope the Bitwarden fetch.\n' >&2
        return 1
      fi
      local folder="${rest%%/*}"
      rest="${rest#*/}"
      local item_name field="password"
      if [[ "$rest" == */* ]]; then
        item_name="${rest%%/*}"
        field="${rest#*/}"
      else
        item_name="$rest"
      fi
      if [[ -z "$folder" || -z "$item_name" ]]; then
        printf 'resolve-env-refs: folder and item name must not be empty: %s\n' "$ref" >&2
        return 1
      fi
      _renv_bw_get "$folder" "$item_name" "$field"
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
#   Two-pass design:
#     Pass 1 — Collect all unique bw:// folder names.
#              Batch-fetch every folder's items (one BW unlock at most).
#              Clear BW_SESSION once all data is in the encrypted cache.
#     Pass 2 — Emit exports, resolving refs from the in-memory cache.
#

resolve_env_file() {
  local file="${1:-.env}"

  if [[ ! -f "$file" ]]; then
    printf 'resolve-env-refs: file not found: %s\n' "$file" >&2
    return 1
  fi

  local line key raw rest folder
  local -a needed_folders=()

  # ── Pass 1: collect unique bw:// folders ─────────────────────────────────
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*$  ]] && continue
    [[ "$line" =~ ^[[:space:]]*#  ]] && continue
    [[ "$line" =~ ^[[:space:]]*(source|export[[:space:]]+[^=]|declare|typeset) ]] && continue

    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      raw="${BASH_REMATCH[3]}"
      if [[ "$raw" =~ ^\"(.*)\"$ ]]; then raw="${BASH_REMATCH[1]}"; fi
      if [[ "$raw" =~ ^\'(.*)\'$ ]]; then raw="${BASH_REMATCH[1]}"; fi

      if [[ "$raw" == bw://* ]]; then
        rest="${raw#bw://}"
        folder="${rest%%/*}"
        if [[ -n "$folder" && "$rest" == */* ]]; then
          _renv_in_array "$folder" "${needed_folders[@]+"${needed_folders[@]}"}" \
            || needed_folders+=("$folder")
        fi
      fi
    fi
  done < "$file"

  # ── Batch-fetch all needed folders (one BW unlock, then session cleared) ──
  local f
  for f in "${needed_folders[@]+"${needed_folders[@]}"}"; do
    _renv_bw_folder_items "$f" || return 1
  done

  # Clear the ephemeral BW session — all data is now in the encrypted cache.
  _renv_bw_clear_session

  # ── Pass 2: emit exports ─────────────────────────────────────────────────
  local resolved
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ ^[[:space:]]*$  ]] && continue
    [[ "$line" =~ ^[[:space:]]*#  ]] && continue
    [[ "$line" =~ ^[[:space:]]*(source|export[[:space:]]+[^=]|declare|typeset) ]] && continue

    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      key="${BASH_REMATCH[2]}"
      raw="${BASH_REMATCH[3]}"

      if [[ "$raw" =~ ^\"(.*)\"$ ]]; then raw="${BASH_REMATCH[1]}"; fi
      if [[ "$raw" =~ ^\'(.*)\'$ ]]; then raw="${BASH_REMATCH[1]}"; fi

      if [[ "$raw" == bw://* || "$raw" == vault://* ]]; then
        resolved=$(_renv_resolve "$raw") || return 1
      else
        resolved="$raw"
      fi

      printf 'export %s=%q\n' "$key" "$resolved"
      _RENV_VARS+=("$key")
    fi
  done < "$file"

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
  _RENV_BW_ITEMS="[]"
  _RENV_BW_SESSION=""
  _RENV_BW_PW_PLAIN=""
  _RENV_CACHE_KEY=""
  _RENV_BW_ACCT_TAG=""
  _RENV_BW_FETCHED_FOLDERS=()
  _RENV_BW_FOLDER_IDS=()
  # Disk cache files are intentionally kept so the next shell can decrypt
  # them with the master password without contacting Bitwarden.
  # Run renv_clear_cache to explicitly remove them.
  _RENV_CACHE_FILES=()
}

# renv_clear_cache — explicitly remove all per-folder disk cache files.
#
#   Removes files created by this session plus any stale renv-*.enc files in
#   /dev/shm that belong to the current user and have exceeded the TTL.
#
renv_clear_cache() {
  local f
  for f in "${_RENV_CACHE_FILES[@]+"${_RENV_CACHE_FILES[@]}"}"; do
    if [[ -f "$f" ]]; then
      rm -f "$f"
      printf 'resolve-env-refs: removed cache %s\n' "$f" >&2
    fi
  done
  _RENV_CACHE_FILES=()

  # Also sweep stale files left by previous sessions.
  local cache_d
  cache_d=$(_renv_cache_dir)
  if [[ -d "$cache_d" ]]; then
    local uid_str="${UID:-$(id -u 2>/dev/null || echo 0)}"
    find "$cache_d" -maxdepth 1 -name 'renv-*.enc' -user "$uid_str" \
      -mmin "+$(( _RENV_CACHE_MAX_AGE / 60 ))" -delete 2>/dev/null || true
  fi
}
