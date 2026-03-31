#!/usr/bin/env bash
# kctx.sh — Ephemeral kubeconfig switching via Vault or Bitwarden
#
# Fetches a kubeconfig into a RAM-backed tmpfile (/dev/shm on Linux) and
# exports KUBECONFIG pointing at it. The file is cleaned up on the next
# call, on kctx_clear, or when the shell exits.
#
# Caching: the kubeconfig is cached in /dev/shm for _KCTX_CACHE_TTL seconds
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
_KCTX_CACHE_TTL=3600  # seconds

trap '_kctx_cleanup' EXIT

_kctx_cleanup() {
  [[ -n "${_KCTX_TMPFILE:-}" ]] && rm -f "$_KCTX_TMPFILE"
}

# ---------------------------------------------------------------------------
# Cache helpers
# ---------------------------------------------------------------------------

_kctx_cache_dir() {
  [[ -d /dev/shm && -w /dev/shm ]] && printf '/dev/shm' || printf '/tmp'
}

_kctx_cache_file() {
  local path="$1"
  local id
  id=$(printf '%s' "$path" | sha256sum 2>/dev/null | cut -c1-16) \
    || id=$(printf '%s' "$path" | openssl dgst -sha256 2>/dev/null | awk '{print substr($NF,1,16)}')
  printf '%s/kctx-%s' "$(_kctx_cache_dir)" "$id"
}

_kctx_cache_valid() {
  local file="$1"
  [[ -f "$file" ]] || return 1
  local now mtime age
  now=$(date +%s)
  mtime=$(stat -c '%Y' "$file" 2>/dev/null) \
    || mtime=$(stat -f '%m' "$file" 2>/dev/null) \
    || return 1
  age=$(( now - mtime ))
  (( age < _KCTX_CACHE_TTL ))
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

  local cache_file from_cache=false
  cache_file=$(_kctx_cache_file "$vault_path")

  if _kctx_cache_valid "$cache_file"; then
    cp "$cache_file" "$tmpfile"
    from_cache=true
  elif [[ "$vault_path" == bw://* ]]; then
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
  else
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

  if ! $from_cache; then
    cp "$tmpfile" "$cache_file"
    chmod 600 "$cache_file"
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
  for f in "$cache_dir"/kctx-*; do
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
