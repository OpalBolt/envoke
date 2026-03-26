#!/usr/bin/env bash
# vault-login.sh — Vault authentication helpers
#
# Usage:
#   source snippets/vault-login.sh
#   vault_login_oidc
#   vault_login_approle "<ROLE_ID>" "<SECRET_ID>"
#   vault_token_info
#   vault_token_renew

set -euo pipefail

: "${VAULT_ADDR:?VAULT_ADDR must be set (e.g. export VAULT_ADDR=https://vault.example.com:8200)}"

# ---------------------------------------------------------------------------
# vault_login_oidc
#   Interactive OIDC login via browser. Writes token to ~/.vault-token.
# ---------------------------------------------------------------------------
vault_login_oidc() {
  local role="${1:-}"
  if [[ -n "$role" ]]; then
    vault login -method=oidc role="$role"
  else
    vault login -method=oidc
  fi
  echo "✅ Logged in via OIDC"
  vault token lookup | grep -E "expire_time|policies"
}

# ---------------------------------------------------------------------------
# vault_login_approle <role_id> <secret_id>
#   Non-interactive AppRole login. Exports VAULT_TOKEN.
# ---------------------------------------------------------------------------
vault_login_approle() {
  local role_id="${1:?Usage: vault_login_approle <role_id> <secret_id>}"
  local secret_id="${2:?Usage: vault_login_approle <role_id> <secret_id>}"

  local token
  token=$(vault write -field=token auth/approle/login \
    role_id="$role_id" \
    secret_id="$secret_id")

  export VAULT_TOKEN="$token"
  echo "✅ Logged in via AppRole. Token TTL:"
  vault token lookup -field=ttl
}

# ---------------------------------------------------------------------------
# vault_login_token <token>
#   Login with a known token (for CI/CD pipelines).
# ---------------------------------------------------------------------------
vault_login_token() {
  local token="${1:?Usage: vault_login_token <token>}"
  export VAULT_TOKEN="$token"
  vault token lookup >/dev/null 2>&1 || {
    echo "❌ Invalid or expired token" >&2
    unset VAULT_TOKEN
    return 1
  }
  echo "✅ Token is valid"
}

# ---------------------------------------------------------------------------
# vault_token_info
#   Print current token details.
# ---------------------------------------------------------------------------
vault_token_info() {
  vault token lookup
}

# ---------------------------------------------------------------------------
# vault_token_renew
#   Renew the current token's TTL.
# ---------------------------------------------------------------------------
vault_token_renew() {
  vault token renew
  echo "✅ Token renewed"
  vault token lookup -field=expire_time
}

# ---------------------------------------------------------------------------
# vault_logout
#   Revoke the current token and unset VAULT_TOKEN.
# ---------------------------------------------------------------------------
vault_logout() {
  vault token revoke -self 2>/dev/null || true
  unset VAULT_TOKEN
  rm -f ~/.vault-token
  echo "✅ Logged out of Vault"
}
