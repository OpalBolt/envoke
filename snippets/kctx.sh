#!/usr/bin/env bash
# kctx.sh — Ephemeral kubeconfig switching via Vault
#
# Fetches a kubeconfig from Vault into a RAM-backed tmpfile (/dev/shm on Linux)
# and exports KUBECONFIG pointing at it. The file is cleaned up on the next
# call, on kctx_clear, or when the shell exits.
#
# Usage:
#   source snippets/kctx.sh
#
#   kctx prod                        # fetches from secret/k8s/prod
#   kctx staging                     # fetches from secret/k8s/staging
#   kctx prod secret/infra/k8s/prod  # explicit Vault path override
#   kctx_status                      # show current context
#   kctx_clear                       # wipe the tmpfile and unset KUBECONFIG
#
# Prerequisites: vault CLI, kubectl (optional — for current-context display)

# Internal state — tracks the active tmpfile so each call can clean up the previous one.
_KCTX_TMPFILE=""

_kctx_cleanup() {
  [[ -n "${_KCTX_TMPFILE:-}" ]] && rm -f "$_KCTX_TMPFILE"
}

# Single EXIT trap registered once at source time.
# This avoids the overwrite-on-repeated-call bug: if each function call set
# trap ... EXIT, the second call would clobber the first trap, leaking the
# previous tmpfile. The module-level variable + single trap avoids that.
trap _kctx_cleanup EXIT

# ---------------------------------------------------------------------------
# kctx <environment> [vault-path]
#   Switch KUBECONFIG to an ephemeral tmpfile backed by Vault.
#
#   environment   Short label used for display and as the default Vault path
#                 suffix: "prod" → secret/k8s/prod.
#   vault-path    Optional. Override the full Vault KV path.
# ---------------------------------------------------------------------------
kctx() {
  local env="${1:?Usage: kctx <environment> [vault-path]}"
  local vault_path="${2:-secret/k8s/${env}}"

  # Clean up previous tmpfile before creating a new one.
  [[ -n "${_KCTX_TMPFILE:-}" ]] && rm -f "$_KCTX_TMPFILE"
  _KCTX_TMPFILE=""

  # Use /dev/shm (RAM-backed tmpfs) on Linux.
  # Fall back to the system temp dir on macOS — not RAM-backed; see the
  # tmpfs mount approach in guides/kubernetes/kubeconfig.md for macOS.
  local tmpfile
  if [[ -d /dev/shm ]]; then
    tmpfile=$(mktemp /dev/shm/kubeconfig-XXXXXX)
  else
    tmpfile=$(mktemp "${TMPDIR:-/tmp}/kubeconfig-XXXXXX")
  fi
  chmod 600 "$tmpfile"

  if ! vault kv get -field=kubeconfig "$vault_path" > "$tmpfile" 2>/dev/null; then
    rm -f "$tmpfile"
    echo "❌ Failed to fetch kubeconfig from Vault: ${vault_path}" >&2
    echo "   Check: vault kv get ${vault_path}" >&2
    return 1
  fi

  if ! grep -q 'apiVersion' "$tmpfile" 2>/dev/null; then
    rm -f "$tmpfile"
    echo "❌ Vault returned data that does not look like a kubeconfig (missing 'apiVersion')" >&2
    return 1
  fi

  _KCTX_TMPFILE="$tmpfile"
  export KUBECONFIG="$tmpfile"

  local storage_label
  [[ -d /dev/shm ]] && storage_label="RAM-backed (/dev/shm)" || storage_label="disk-based (/dev/shm not available)"

  echo "✅ Switched to ${env} — ${storage_label}"
  echo "   Vault path : ${vault_path}"
  echo "   Context    : $(kubectl config current-context 2>/dev/null || echo '(kubectl not found)')"
}

# ---------------------------------------------------------------------------
# kctx_clear
#   Remove the active tmpfile and unset KUBECONFIG.
# ---------------------------------------------------------------------------
kctx_clear() {
  _kctx_cleanup
  _KCTX_TMPFILE=""
  unset KUBECONFIG
  echo "✅ KUBECONFIG cleared"
}

# ---------------------------------------------------------------------------
# kctx_status
#   Show the active KUBECONFIG and current kubectl context.
# ---------------------------------------------------------------------------
kctx_status() {
  if [[ -z "${KUBECONFIG:-}" ]]; then
    echo "No KUBECONFIG set (kubectl uses ~/.kube/config)"
    return
  fi

  echo "KUBECONFIG : ${KUBECONFIG}"

  if [[ -n "${_KCTX_TMPFILE:-}" ]]; then
    if [[ -d /dev/shm ]]; then
      echo "storage    : RAM-backed (/dev/shm) — never written to disk"
    else
      echo "storage    : disk-based (/dev/shm not available on this OS)"
    fi
  fi

  echo "context    : $(kubectl config current-context 2>/dev/null || echo '(kubectl not found or no context set)')"
}
