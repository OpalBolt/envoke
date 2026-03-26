#!/usr/bin/env bash
# bw-get-secret.sh — Bitwarden CLI secret retrieval helpers
#
# Usage:
#   source snippets/bw-get-secret.sh
#   bw_ensure_unlocked
#   bw_get_password "my-service"
#   bw_get_field "my-service" "api_key"
#   bw_get_note "my-certificate"

set -euo pipefail

# ---------------------------------------------------------------------------
# bw_ensure_unlocked
#   Ensures BW_SESSION is set and valid. Prompts for unlock if needed.
#   Exports BW_SESSION.
# ---------------------------------------------------------------------------
bw_ensure_unlocked() {
  # If already set, verify it's still valid
  if [[ -n "${BW_SESSION:-}" ]]; then
    if bw status --session "$BW_SESSION" 2>/dev/null | grep -q '"status":"unlocked"'; then
      return 0
    fi
  fi

  # Unlock interactively
  echo "🔐 Bitwarden vault is locked. Unlocking..." >&2
  BW_SESSION=$(bw unlock --raw) || {
    echo "❌ Failed to unlock Bitwarden vault" >&2
    return 1
  }
  export BW_SESSION
  bw sync --session "$BW_SESSION" >/dev/null
}

# ---------------------------------------------------------------------------
# bw_get_password <item_name>
#   Retrieve the password for a login item by name.
# ---------------------------------------------------------------------------
bw_get_password() {
  local item_name="${1:?Usage: bw_get_password <item_name>}"
  bw_ensure_unlocked
  local result
  result=$(bw get password "$item_name" --session "$BW_SESSION" 2>/dev/null) || {
    echo "❌ Item '$item_name' not found in Bitwarden" >&2
    return 1
  }
  printf '%s' "$result"
}

# ---------------------------------------------------------------------------
# bw_get_username <item_name>
#   Retrieve the username for a login item by name.
# ---------------------------------------------------------------------------
bw_get_username() {
  local item_name="${1:?Usage: bw_get_username <item_name>}"
  bw_ensure_unlocked
  bw get username "$item_name" --session "$BW_SESSION" 2>/dev/null
}

# ---------------------------------------------------------------------------
# bw_get_field <item_name> <field_name>
#   Retrieve a custom field value from an item.
# ---------------------------------------------------------------------------
bw_get_field() {
  local item_name="${1:?Usage: bw_get_field <item_name> <field_name>}"
  local field_name="${2:?Usage: bw_get_field <item_name> <field_name>}"
  bw_ensure_unlocked
  local value
  value=$(bw get item "$item_name" --session "$BW_SESSION" 2>/dev/null | \
    jq -re --arg f "$field_name" '.fields[]? | select(.name == $f) | .value') || true
  if [[ -z "$value" ]]; then
    echo "❌ Field '$field_name' not found or empty in item '$item_name'" >&2
    return 1
  fi
  printf '%s' "$value"
}

# ---------------------------------------------------------------------------
# bw_get_note <item_name>
#   Retrieve the notes field of an item (useful for certificates, keys).
# ---------------------------------------------------------------------------
bw_get_note() {
  local item_name="${1:?Usage: bw_get_note <item_name>}"
  bw_ensure_unlocked
  bw get notes "$item_name" --session "$BW_SESSION" 2>/dev/null
}

# ---------------------------------------------------------------------------
# bw_get_totp <item_name>
#   Retrieve the current TOTP code for an item.
# ---------------------------------------------------------------------------
bw_get_totp() {
  local item_name="${1:?Usage: bw_get_totp <item_name>}"
  bw_ensure_unlocked
  bw get totp "$item_name" --session "$BW_SESSION" 2>/dev/null
}

# ---------------------------------------------------------------------------
# bw_lock
#   Lock the vault and unset the session key.
# ---------------------------------------------------------------------------
bw_lock() {
  bw lock >/dev/null 2>&1 || true
  unset BW_SESSION
  echo "🔒 Bitwarden vault locked"
}
