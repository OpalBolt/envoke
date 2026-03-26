#!/usr/bin/env bash
# vault-client.sh — Read and write secrets using the Vault CLI.
#
# Prerequisites:
#   export VAULT_ADDR="https://vault.example.com:8200"
#   export VAULT_TOKEN="<YOUR_VAULT_TOKEN>"  # or run: vault login
#
# Usage:
#   ./vault-client.sh write secret/myproject/db password "<MY_PASSWORD>"
#   ./vault-client.sh read  secret/myproject/db
#   ./vault-client.sh get   secret/myproject/db password
#   ./vault-client.sh list  secret/myproject/
#   ./vault-client.sh delete secret/myproject/db

set -euo pipefail

: "${VAULT_ADDR:?VAULT_ADDR must be set}"

usage() {
  cat >&2 << 'EOF'
Usage:
  vault-client.sh write  <path> <key> <value> [<key> <value> ...]
  vault-client.sh read   <path>
  vault-client.sh get    <path> <field>
  vault-client.sh list   <path>
  vault-client.sh delete <path>
  vault-client.sh env    <path>   # export all fields as env vars

EOF
  exit 1
}

[[ $# -lt 2 ]] && usage

CMD="$1"
PATH_ARG="$2"
shift 2

# ---------------------------------------------------------------------------
cmd_write() {
  # Args: key value [key value ...]
  [[ $(( $# % 2 )) -ne 0 ]] && { echo "❌ write requires pairs of key value arguments" >&2; exit 1; }

  local kv_args=()
  while [[ $# -gt 0 ]]; do
    kv_args+=("${1}=${2}")
    shift 2
  done

  vault kv put "$PATH_ARG" "${kv_args[@]}"
  echo "✅ Secret written to: $PATH_ARG"
}

# ---------------------------------------------------------------------------
cmd_read() {
  vault kv get "$PATH_ARG"
}

# ---------------------------------------------------------------------------
cmd_get() {
  local field="${1:?Usage: vault-client.sh get <path> <field>}"
  vault kv get -field="$field" "$PATH_ARG"
}

# ---------------------------------------------------------------------------
cmd_list() {
  vault kv list "$PATH_ARG"
}

# ---------------------------------------------------------------------------
cmd_delete() {
  vault kv delete "$PATH_ARG"
  echo "🗑️  Secret deleted (soft): $PATH_ARG"
}

# ---------------------------------------------------------------------------
# cmd_env: Export all fields from a KV secret as environment variables.
# Field names are uppercased. Designed to be used with eval:
#
#   eval "$(./vault-client.sh env secret/myproject/dev)"
# ---------------------------------------------------------------------------
cmd_env() {
  vault kv get -format=json "$PATH_ARG" | \
    jq -r '.data.data // .data | to_entries[] | "export \(.key | ascii_upcase)=\(.value | @sh)"'
}

# ---------------------------------------------------------------------------
case "$CMD" in
  write)  cmd_write "$@" ;;
  read)   cmd_read ;;
  get)    cmd_get "$@" ;;
  list)   cmd_list ;;
  delete) cmd_delete ;;
  env)    cmd_env ;;
  *)
    echo "❌ Unknown command: $CMD" >&2
    usage
    ;;
esac
