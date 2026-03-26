#!/usr/bin/env bash
# inject-env.sh — Inject secrets from Vault or Bitwarden into the environment
#                 of a child process without persisting them to disk or history.
#
# Usage:
#   ./inject-env.sh vault secret/myproject/dev -- node server.js
#   ./inject-env.sh bitwarden "my-service" -- python app.py
#   ./inject-env.sh vault-json secret/myproject/dev -- ./start.sh

set -euo pipefail

usage() {
  cat >&2 << 'EOF'
Usage:
  inject-env.sh vault <vault-path> -- <command> [args...]
  inject-env.sh vault-json <vault-path> -- <command> [args...]
  inject-env.sh bitwarden <item-name> -- <command> [args...]

Backends:
  vault         Read all fields from a Vault KV path, export as env vars
                (field names are uppercased)
  vault-json    Same as vault — alias for clarity
  bitwarden     Read all fields from a Bitwarden item, export as env vars

Examples:
  inject-env.sh vault secret/myproject/dev -- node server.js
  inject-env.sh bitwarden "prod-api-service" -- ./start.sh
EOF
  exit 1
}

# ---------------------------------------------------------------------------
# inject_from_vault <path> <command...>
# ---------------------------------------------------------------------------
inject_from_vault() {
  local vault_path="$1"
  shift

  : "${VAULT_ADDR:?VAULT_ADDR must be set}"

  if ! vault token lookup >/dev/null 2>&1; then
    echo "❌ Not authenticated to Vault. Run: vault login" >&2
    exit 1
  fi

  local json
  json=$(vault kv get -format=json "$vault_path") || {
    echo "❌ Failed to read '$vault_path' from Vault" >&2
    exit 1
  }

  # Use NUL-delimited KEY=VALUE records to preserve multiline secret values
  local -a env_vars=()
  while IFS= read -r -d $'\0' kv; do
    env_vars+=("$kv")
  done < <(echo "$json" | jq -rj '.data.data // .data | to_entries[] | "\(.key | ascii_upcase)=\(.value)\u0000"')

  if [[ ${#env_vars[@]} -eq 0 ]]; then
    echo "⚠️  No secrets found at '$vault_path'" >&2
  fi

  exec env "${env_vars[@]}" "$@"
}

# ---------------------------------------------------------------------------
# inject_from_bitwarden <item_name> <command...>
# ---------------------------------------------------------------------------
inject_from_bitwarden() {
  local item_name="$1"
  shift

  if ! command -v bw >/dev/null 2>&1; then
    echo "❌ Bitwarden CLI (bw) is not installed" >&2
    exit 1
  fi

  if [[ -z "${BW_SESSION:-}" ]]; then
    echo "🔐 Unlocking Bitwarden vault..." >&2
    BW_SESSION=$(bw unlock --raw) || {
      echo "❌ Failed to unlock Bitwarden vault" >&2
      exit 1
    }
    export BW_SESSION
  fi

  local item_json
  item_json=$(bw get item "$item_name" --session "$BW_SESSION" 2>/dev/null) || {
    echo "❌ Item '$item_name' not found in Bitwarden" >&2
    exit 1
  }

  local -a env_vars=()

  # Login username and password
  local username password
  username=$(echo "$item_json" | jq -r '.login.username // empty')
  password=$(echo "$item_json" | jq -r '.login.password // empty')
  [[ -n "$username" ]] && env_vars+=("USERNAME=${username}")
  [[ -n "$password" ]] && env_vars+=("PASSWORD=${password}")

  # Custom fields — NUL-delimited to handle multiline values
  while IFS= read -r -d $'\0' kv; do
    env_vars+=("$kv")
  done < <(echo "$item_json" | jq -rj '.fields[]? | "\(.name | ascii_upcase)=\(.value)\u0000"')

  if [[ ${#env_vars[@]} -eq 0 ]]; then
    echo "⚠️  No usable fields found in Bitwarden item '$item_name'" >&2
  fi

  exec env "${env_vars[@]}" "$@"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
[[ $# -lt 3 ]] && usage

BACKEND="$1"
SECRET_REF="$2"
shift 2

# Consume the "--" separator
if [[ "${1:-}" == "--" ]]; then
  shift
fi

[[ $# -eq 0 ]] && { echo "❌ No command specified after '--'" >&2; usage; }

case "$BACKEND" in
  vault|vault-json)
    inject_from_vault "$SECRET_REF" "$@"
    ;;
  bitwarden|bw)
    inject_from_bitwarden "$SECRET_REF" "$@"
    ;;
  *)
    echo "❌ Unknown backend: '$BACKEND'. Use 'vault' or 'bitwarden'" >&2
    usage
    ;;
esac
