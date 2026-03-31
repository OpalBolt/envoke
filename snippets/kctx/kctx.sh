#!/usr/bin/env bash
# kctx.sh — Ephemeral kubeconfig switching via Vault or Bitwarden
#
# Fetches a kubeconfig into a RAM-backed tmpfile (/dev/shm on Linux) and
# exports KUBECONFIG pointing at it. The file is cleaned up on the next
# call, on kctx_clear, or when the shell exits.
#
# Caching: the kubeconfig is cached in /dev/shm for _KCTX_CACHE_MAX_AGE seconds
# (default 1 hour) so repeated calls don't re-fetch from Vault/Bitwarden.
#
# Usage:
#   source ~/.config/kctx/kctx.sh
#
#   kctx prod                          # fetches from vault secret/k8s/prod
#   kctx prod secret/infra/k8s/prod    # explicit Vault KV path
#   kctx prod bw://k8s-prod            # kubeconfig from Bitwarden item (kubeconfig field)
#   kctx prod bw://k8s-prod/field:cfg  # kubeconfig from Bitwarden custom field "cfg"
#   kctx_status                        # show current context
#   kctx_clear                         # wipe the tmpfile and unset KUBECONFIG
#   kctx_cache_clear                   # delete all cached kubeconfigs from /dev/shm
#
# Prerequisites:
#   vault              — for vault:// paths (VAULT_ADDR + VAULT_TOKEN must be set)
#   bw + jq            — for bw:// paths (BW_SESSION must be set)
#   kubectl (optional) — for current-context display

_KCTX_TMPFILE=""
_KCTX_CACHE_MAX_AGE=3600  # seconds

trap '_kctx_cleanup' EXIT

_kctx_cleanup() {
  [[ -n "${_KCTX_TMPFILE:-}" ]] && rm -f "$_KCTX_TMPFILE"
}

# ---------------------------------------------------------------------------
# Cache helpers — RAM-backed, AES-256-CBC encrypted (same pattern as
# resolve-env-refs.sh).
#
# Naming:  /dev/shm/kctx-<first16 of SHA-256(vault_path:session)>.enc
#   - Unique per path+session → no cross-environment or cross-login collisions
#   - Encrypted with AES-256-CBC + PBKDF2, key = SHA-256(session_token)
#   - chmod 600; discarded after _KCTX_CACHE_MAX_AGE seconds
# ---------------------------------------------------------------------------

_kctx_cache_dir() {
  [[ -d /dev/shm && -w /dev/shm ]] && printf '/dev/shm' || printf '/tmp'
}

_kctx_cache_file() {
  local path="$1" session="$2"
  local id
  id=$(printf '%s:%s' "$path" "$session" | sha256sum 2>/dev/null | cut -c1-16) \
    || id=$(printf '%s:%s' "$path" "$session" | openssl dgst -sha256 2>/dev/null | awk '{print substr($NF,1,16)}')
  printf '%s/kctx-%s.enc' "$(_kctx_cache_dir)" "$id"
}

_kctx_cache_key() {
  local session="$1"
  printf '%s' "$session" | sha256sum 2>/dev/null | awk '{print $1}' \
    || printf '%s' "$session" | openssl dgst -sha256 2>/dev/null | awk '{print $NF}'
}

_kctx_cache_stale() {
  local file="$1"
  local now mtime age
  now=$(date +%s)
  mtime=$(stat -c '%Y' "$file" 2>/dev/null) \
    || mtime=$(stat -f '%m' "$file" 2>/dev/null) \
    || { printf 'kctx: cannot stat cache file\n' >&2; return 0; }
  age=$(( now - mtime ))
  (( age > _KCTX_CACHE_MAX_AGE ))
}

# ---------------------------------------------------------------------------
# Bitwarden fetch
# ---------------------------------------------------------------------------

_kctx_bw_fetch() {
  local item_name="$1"
  local field="${2:-kubeconfig}"

  if [[ -z "${BW_SESSION:-}" ]]; then
    printf 'kctx: BW_SESSION is not set.\n' >&2
    printf '  Run:  export BW_SESSION=$(bw unlock --raw)\n' >&2
    return 1
  fi

  if ! command -v bw >/dev/null 2>&1; then
    printf 'kctx: bw CLI not found\n' >&2
    return 1
  fi
  if ! command -v jq >/dev/null 2>&1; then
    printf 'kctx: jq not found\n' >&2
    return 1
  fi

  local value
  if [[ "$field" == "notes" ]]; then
    value=$(bw get notes "$item_name" --session "$BW_SESSION" 2>/dev/null)
  else
    value=$(bw get item "$item_name" --session "$BW_SESSION" 2>/dev/null \
      | jq -r ".fields[]? | select(.name==\"$field\") | .value")
  fi

  if [[ -z "$value" ]]; then
    printf 'kctx: Bitwarden item "%s" field "%s" not found or empty\n' "$item_name" "$field" >&2
    printf '  Check: bw get item "%s" --session "$BW_SESSION"\n' "$item_name" >&2
    return 1
  fi

  printf '%s' "$value"
}

# ---------------------------------------------------------------------------
# kctx <environment> [vault-path|bw://...]
# ---------------------------------------------------------------------------
kctx() {
  local env="${1:?Usage: kctx <environment> [vault-path|bw://item]}"
  local vault_path="${2:-secret/k8s/${env}}"

  [[ -n "${_KCTX_TMPFILE:-}" ]] && rm -f "$_KCTX_TMPFILE"
  _KCTX_TMPFILE=""

  local tmpfile
  if [[ -d /dev/shm ]]; then
    tmpfile=$(mktemp /dev/shm/kubeconfig-XXXXXX)
  else
    tmpfile=$(mktemp "${TMPDIR:-/tmp}/kubeconfig-XXXXXX")
  fi
  chmod 600 "$tmpfile"

  # Determine the session token used as encryption key (Vault or BW).
  local session
  if [[ "$vault_path" == bw://* ]]; then
    session="${BW_SESSION:-}"
  else
    session="${VAULT_TOKEN:-}"
  fi

  local cache_file cache_key from_cache=false
  if [[ -n "$session" ]]; then
    cache_file=$(_kctx_cache_file "$vault_path" "$session")
    cache_key=$(_kctx_cache_key "$session")

    if [[ -f "$cache_file" ]]; then
      if _kctx_cache_stale "$cache_file"; then
        printf 'kctx: cache expired, re-fetching...\n' >&2
        rm -f "$cache_file"
      else
        local decrypted
        decrypted=$(openssl enc -d -aes-256-cbc -pbkdf2 \
          -pass "pass:${cache_key}" -in "$cache_file" 2>/dev/null) || {
          printf 'kctx: cache decrypt failed (stale session?), re-fetching...\n' >&2
          rm -f "$cache_file"
          decrypted=""
        }
        if [[ -n "$decrypted" ]]; then
          printf '%s' "$decrypted" > "$tmpfile"
          from_cache=true
        fi
      fi
    fi
  fi

  if ! $from_cache && [[ "$vault_path" == bw://* ]]; then
    local bw_ref="${vault_path#bw://}"
    local item_name field
    if [[ "$bw_ref" == *"/field:"* ]]; then
      item_name="${bw_ref%%/field:*}"
      field="${bw_ref##*/field:}"
    else
      item_name="$bw_ref"
      field="kubeconfig"
    fi

    if ! _kctx_bw_fetch "$item_name" "$field" > "$tmpfile"; then
      rm -f "$tmpfile"
      return 1
    fi
  elif ! $from_cache; then
    if ! vault kv get -field=kubeconfig "$vault_path" > "$tmpfile" 2>/dev/null; then
      rm -f "$tmpfile"
      printf '❌ Failed to fetch kubeconfig from Vault: %s\n' "$vault_path" >&2
      printf '   Check: vault kv get %s\n' "$vault_path" >&2
      return 1
    fi
  fi

  if ! grep -q 'apiVersion' "$tmpfile" 2>/dev/null; then
    rm -f "$tmpfile"
    printf '❌ Fetched data does not look like a kubeconfig (missing apiVersion)\n' >&2
    return 1
  fi

  if ! $from_cache && [[ -n "${cache_file:-}" ]]; then
    printf '%s' "$(cat "$tmpfile")" \
      | openssl enc -aes-256-cbc -pbkdf2 \
          -pass "pass:${cache_key}" -out "$cache_file" 2>/dev/null \
      && chmod 600 "$cache_file" \
      || printf 'kctx: warning: could not write cache (%s)\n' "$cache_file" >&2
  fi

  _KCTX_TMPFILE="$tmpfile"
  export KUBECONFIG="$tmpfile"

  local storage_label
  [[ -d /dev/shm ]] && storage_label="RAM-backed (/dev/shm)" || storage_label="disk-based (/dev/shm not available)"

  local source_label
  $from_cache && source_label=" (cached)" || source_label=""

  printf '✅ Switched to %s%s — %s\n' "$env" "$source_label" "$storage_label"
  printf '   Path    : %s\n' "$vault_path"
  printf '   Context : %s\n' "$(kubectl config current-context 2>/dev/null || printf '(kubectl not found)')"
}

# ---------------------------------------------------------------------------
# kctx_clear — unset KUBECONFIG and remove the active tmpfile
# ---------------------------------------------------------------------------
kctx_clear() {
  _kctx_cleanup
  _KCTX_TMPFILE=""
  unset KUBECONFIG
  printf '✅ KUBECONFIG cleared\n'
}

# ---------------------------------------------------------------------------
# kctx_cache_clear — delete all kctx cache files from /dev/shm (or /tmp)
# ---------------------------------------------------------------------------
kctx_cache_clear() {
  local cache_dir
  cache_dir=$(_kctx_cache_dir)
  local count=0
  for f in "$cache_dir"/kctx-*.enc; do
    [[ -f "$f" ]] || continue
    rm -f "$f"
    (( count++ )) || true
  done
  printf '✅ Removed %d cached kubeconfig(s) from %s\n' "$count" "$cache_dir"
}

# ---------------------------------------------------------------------------
# kctx_status — show active KUBECONFIG and current kubectl context
# ---------------------------------------------------------------------------
kctx_status() {
  if [[ -z "${KUBECONFIG:-}" ]]; then
    printf 'No KUBECONFIG set (kubectl uses ~/.kube/config)\n'
    return
  fi

  printf 'KUBECONFIG : %s\n' "$KUBECONFIG"

  if [[ -n "${_KCTX_TMPFILE:-}" ]]; then
    [[ -d /dev/shm ]] \
      && printf 'storage    : RAM-backed (/dev/shm) — never written to disk\n' \
      || printf 'storage    : disk-based (/dev/shm not available on this OS)\n'
  fi

  printf 'context    : %s\n' "$(kubectl config current-context 2>/dev/null || printf '(kubectl not found or no context set)')"
}
