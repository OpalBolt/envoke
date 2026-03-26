#!/usr/bin/env bash
# inject.sh — Secret injector agent
#
# Injects secrets from Vault or Bitwarden into the environment of a child process.
#
# Usage:
#   ./inject.sh vault  <vault-path> [<vault-path2> ...] -- <command> [args...]
#   ./inject.sh bitwarden <item-name> -- <command> [args...]
#   ./inject.sh vault  <vault-path> --dry-run -- <command>

set -euo pipefail

usage() {
  cat >&2 << 'EOF'
Usage:
  inject.sh vault     <vault-path> [<path2> ...] [--dry-run] -- <command> [args...]
  inject.sh bitwarden <item-name>                [--dry-run] -- <command> [args...]

Backends:
  vault       Read all KV fields from one or more Vault paths
  bitwarden   Read login credentials + custom fields from a Bitwarden item

EOF
  exit 1
}

[[ $# -lt 3 ]] && usage

BACKEND="$1"
shift

DRY_RUN=0
SOURCES=()

# Collect source paths/items until "--" separator
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --)        shift; break ;;
    *)         SOURCES+=("$1"); shift ;;
  esac
done

[[ ${#SOURCES[@]} -eq 0 ]] && { echo "❌ No secret source specified" >&2; usage; }
[[ $# -eq 0 ]] && { echo "❌ No command specified after '--'" >&2; usage; }

CMD=("$@")

# ---------------------------------------------------------------------------
collect_vault_env() {
  : "${VAULT_ADDR:?VAULT_ADDR must be set}"

  if ! vault token lookup >/dev/null 2>&1; then
    echo "❌ Not authenticated to Vault. Run: vault login" >&2
    exit 1
  fi

  for path in "${SOURCES[@]}"; do
    local json
    json=$(vault kv get -format=json "$path" 2>/dev/null) || {
      echo "❌ Failed to read '$path' from Vault" >&2
      exit 1
    }
    # Use NUL-delimited output to safely handle multiline secret values
    echo "$json" | jq -rj '.data.data // .data | to_entries[] | "\(.key | ascii_upcase)=\(.value)\u0000"'
  done
}

collect_bitwarden_env() {
  local item="${SOURCES[0]}"

  if [[ -z "${BW_SESSION:-}" ]]; then
    echo "🔐 Unlocking Bitwarden vault..." >&2
    BW_SESSION=$(bw unlock --raw 2>/dev/null) || {
      echo "❌ Failed to unlock Bitwarden vault" >&2
      exit 1
    }
    export BW_SESSION
  fi

  local json
  json=$(bw get item "$item" --session "$BW_SESSION" --nointeraction 2>/dev/null) || {
    echo "❌ Item '$item' not found in Bitwarden" >&2
    exit 1
  }

  local username password
  username=$(echo "$json" | jq -r '.login.username // empty')
  password=$(echo "$json" | jq -r '.login.password // empty')
  [[ -n "$username" ]] && printf 'USERNAME=%s\0' "$username"
  [[ -n "$password" ]] && printf 'PASSWORD=%s\0' "$password"

  # Custom fields — NUL-delimited to handle multiline values
  echo "$json" | jq -rj '.fields[]? | "\(.name | ascii_upcase)=\(.value)\u0000"'
}

# ---------------------------------------------------------------------------
# Collect env vars using NUL-delimited output to handle multiline secret values
ENV_ARRAY=()
while IFS= read -r -d $'\0' kv; do
  ENV_ARRAY+=("$kv")
done < <(
  case "$BACKEND" in
    vault)     collect_vault_env ;;
    bitwarden) collect_bitwarden_env ;;
    *)         echo "❌ Unknown backend: $BACKEND" >&2; usage ;;
  esac
)

if [[ ${#ENV_ARRAY[@]} -eq 0 ]]; then
  echo "⚠️  No secrets retrieved — proceeding with empty env" >&2
fi

if [[ $DRY_RUN -eq 1 ]]; then
  echo "🔍 Dry run — would export:"
  for pair in "${ENV_ARRAY[@]}"; do
    key="${pair%%=*}"
    echo "  export ${key}=<masked>"
  done
  echo "  Command: ${CMD[*]}"
  exit 0
fi

exec env "${ENV_ARRAY[@]}" "${CMD[@]}"
