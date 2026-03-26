#!/usr/bin/env bash
# bitwarden-client.sh — Retrieve secrets from Bitwarden via the bw CLI.
#
# Prerequisites:
#   bw login (or bw login --apikey)
#   export BW_SESSION=$(bw unlock --raw)
#
# Usage:
#   ./bitwarden-client.sh get-password "my-service"
#   ./bitwarden-client.sh get-field "my-service" api_key
#   ./bitwarden-client.sh get-note "my-cert"
#   ./bitwarden-client.sh list "github"
#   ./bitwarden-client.sh env "my-service"

set -euo pipefail

usage() {
  cat >&2 << 'EOF'
Usage:
  bitwarden-client.sh get-password <item-name>
  bitwarden-client.sh get-username <item-name>
  bitwarden-client.sh get-field    <item-name> <field-name>
  bitwarden-client.sh get-note     <item-name>
  bitwarden-client.sh get-totp     <item-name>
  bitwarden-client.sh list         [search-term]
  bitwarden-client.sh env          <item-name>   # export login + custom fields as env vars

EOF
  exit 1
}

[[ $# -lt 1 ]] && usage

CMD="$1"
shift

# ---------------------------------------------------------------------------
# Ensure vault is unlocked
# ---------------------------------------------------------------------------
ensure_unlocked() {
  if [[ -z "${BW_SESSION:-}" ]]; then
    echo "🔐 Bitwarden vault is locked. Unlocking..." >&2
    BW_SESSION=$(bw unlock --raw 2>/dev/null) || {
      echo "❌ Failed to unlock Bitwarden vault" >&2
      exit 1
    }
    export BW_SESSION
    bw sync --session "$BW_SESSION" >/dev/null
  fi
}

bw_run() {
  ensure_unlocked
  bw "$@" --session "$BW_SESSION" --nointeraction 2>/dev/null
}

# ---------------------------------------------------------------------------
cmd_get_password() {
  local item="${1:?Usage: bitwarden-client.sh get-password <item-name>}"
  bw_run get password "$item"
}

cmd_get_username() {
  local item="${1:?Usage: bitwarden-client.sh get-username <item-name>}"
  bw_run get username "$item"
}

cmd_get_field() {
  local item="${1:?Usage: bitwarden-client.sh get-field <item-name> <field-name>}"
  local field="${2:?Usage: bitwarden-client.sh get-field <item-name> <field-name>}"
  local value
  value=$(bw_run get item "$item" | jq -re --arg f "$field" '.fields[]? | select(.name == $f) | .value' 2>/dev/null || true)
  if [[ -z "$value" ]]; then
    echo "❌ Field '$field' not found or empty in item '$item'" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

cmd_get_note() {
  local item="${1:?Usage: bitwarden-client.sh get-note <item-name>}"
  bw_run get notes "$item"
}

cmd_get_totp() {
  local item="${1:?Usage: bitwarden-client.sh get-totp <item-name>}"
  bw_run get totp "$item"
}

cmd_list() {
  local search="${1:-}"
  if [[ -n "$search" ]]; then
    bw_run list items --search "$search" | jq -r '.[] | "\(.id)\t\(.name)"'
  else
    bw_run list items | jq -r '.[] | "\(.id)\t\(.name)"'
  fi
}

# ---------------------------------------------------------------------------
# cmd_env: Export login credentials + custom fields as environment variables.
# Field names are uppercased. Use with eval:
#
#   eval "$(./bitwarden-client.sh env "my-service")"
# ---------------------------------------------------------------------------
cmd_env() {
  local item="${1:?Usage: bitwarden-client.sh env <item-name>}"
  ensure_unlocked

  local json
  json=$(bw get item "$item" --session "$BW_SESSION" --nointeraction 2>/dev/null)

  local username password
  username=$(echo "$json" | jq -r '.login.username // empty')
  password=$(echo "$json" | jq -r '.login.password // empty')

  [[ -n "$username" ]] && echo "export USERNAME=$(printf '%q' "$username")"
  [[ -n "$password" ]] && echo "export PASSWORD=$(printf '%q' "$password")"

  # Custom fields
  echo "$json" | jq -r '.fields[]? | "export \(.name | ascii_upcase)=\(.value | @sh)"'
}

# ---------------------------------------------------------------------------
case "$CMD" in
  get-password) cmd_get_password "$@" ;;
  get-username) cmd_get_username "$@" ;;
  get-field)    cmd_get_field "$@" ;;
  get-note)     cmd_get_note "$@" ;;
  get-totp)     cmd_get_totp "$@" ;;
  list)         cmd_list "$@" ;;
  env)          cmd_env "$@" ;;
  *)
    echo "❌ Unknown command: $CMD" >&2
    usage
    ;;
esac
