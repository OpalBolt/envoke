#!/usr/bin/env bash
# resolve-env-refs.sh — Resolve bw:// and vault:// references in a .env file.
#
# Instead of storing actual secrets in .env files, store references:
#
#   DATABASE_URL=bw://prod-db/password
#   DATABASE_USER=bw://prod-db/username
#   STRIPE_KEY=bw://stripe-api/field:api_key
#   VAULT_TOKEN=vault://secret/myproject/stripe#secret_key
#
# ─────────────────────────────────────────────────────────────────────────────
# QUICKSTART — two patterns
# ─────────────────────────────────────────────────────────────────────────────
#
# Pattern 1 — direnv (recommended; security-pinned, automatic cleanup on cd away)
#
#   Pin the script to a specific commit SHA and validate with an SRI hash.
#   Generate the hash: shasum -a 256 resolve-env-refs.sh | awk '{print $1}' \
#                        | xxd -r -p | base64
#
#   # .envrc
#   source_url "https://raw.githubusercontent.com/eficode/secure-handling-of-secrets/<SHA>/snippets/resolve-env-refs.sh" \
#     "sha256-<HASH>"
#   source <(resolve_env_file .env)
#
#   Then: direnv allow .
#
# Pattern 2 — self-loading .env (standalone shell: source .env)
#
#   Line 1 fetches and sources this script, resolves all bw:// and vault://
#   references in the file itself, exports resolved values, then returns early
#   so the raw reference strings below are never executed as shell assignments.
#   An unload_env() cleanup function is defined and registered on EXIT.
#   If a different .env was previously loaded, it is unloaded first.
#
#   Works in bash and zsh — no shell-specific syntax required in the .env file.
#
#   # .env
#   source <(curl -fsSL "https://raw.githubusercontent.com/eficode/secure-handling-of-secrets/<SHA>/snippets/resolve-env-refs.sh") \
#     && declare -f _load_self_env &>/dev/null \
#     && _load_self_env \
#     && return 0 2>/dev/null; true
#
#   DATABASE_URL=bw://prod-db/password
#   STRIPE_KEY=bw://stripe-api/field:api_key
#   VAULT_TOKEN=vault://secret/myproject/app#token
#
#   Then: source .env          # resolves refs, sets EXIT cleanup trap
#         unload_env           # manual cleanup (also fires automatically on EXIT)
#
#   ⚠️  The EXIT trap from resolve_env_file replaces any pre-existing EXIT trap.
#       To chain with an existing trap: trap 'unload_env; your_existing_cleanup' EXIT
#
# ─────────────────────────────────────────────────────────────────────────────
# USAGE MODES (when used as a script, not sourced)
# ─────────────────────────────────────────────────────────────────────────────
#
#   Mode 1 — exec (recommended, no shell injection risk):
#     ./snippets/resolve-env-refs.sh .env.example -- node server.js
#     Resolves references and execs the command with the env vars set.
#     Resolved values never enter your shell; no eval needed.
#
#   Mode 2 — source (safe for loading into current shell):
#     source <(./snippets/resolve-env-refs.sh .env.example)
#     Outputs shell-escaped "export KEY=VALUE" lines — use source, NOT eval.
#
# ⚠️  NEVER use:  eval "$(./snippets/resolve-env-refs.sh .env.example)"
#     eval re-interprets secret values as shell code, enabling injection attacks.
#     Use "source <(...)" or exec mode instead.
#
# ─────────────────────────────────────────────────────────────────────────────
# Reference formats:
#
#   Bitwarden Password Manager (bw CLI):
#     bw://item-name              → password field (default)
#     bw://item-name/password     → password field
#     bw://item-name/username     → username field
#     bw://item-name/note         → notes field
#     bw://item-name/field:fname  → custom field named "fname"
#
#   HashiCorp Vault KV v2:
#     vault://secret/path         → all fields (one export per field)
#     vault://secret/path#field   → single field value
#
# Prerequisites:
#   Bitwarden: bw CLI installed; BW_SESSION exported or vault will be unlocked interactively
#   Vault:     vault CLI installed; VAULT_ADDR and VAULT_TOKEN set

set -euo pipefail

# ---------------------------------------------------------------------------
# _bw_ensure_session
#   Ensures BW_SESSION is set and exports it.
#   Handles all three vault states:
#     unauthenticated → bw login (interactive, or API key if BW_CLIENTID/BW_CLIENTSECRET set)
#     locked          → bw unlock (interactive, or --passwordenv BW_PASSWORD if set)
#     unlocked        → re-unlock to obtain a fresh session token
# ---------------------------------------------------------------------------
_bw_ensure_session() {
  if [[ -n "${BW_SESSION:-}" ]]; then
    return 0
  fi

  if ! command -v bw >/dev/null 2>&1; then
    echo "❌ bw CLI not found. Install: npm install -g @bitwarden/cli" >&2
    return 1
  fi

  local bw_status
  bw_status=$(bw status 2>/dev/null | jq -r '.status // empty' 2>/dev/null) || bw_status=""

  if [[ "$bw_status" == "unauthenticated" ]]; then
    echo "🔑 Logging in to Bitwarden..." >&2
    if [[ -n "${BW_CLIENTID:-}" && -n "${BW_CLIENTSECRET:-}" ]]; then
      bw login --apikey 2>/dev/null || {
        echo "❌ API key login failed. Check BW_CLIENTID / BW_CLIENTSECRET." >&2
        return 1
      }
    else
      bw login || {
        echo "❌ Failed to log in to Bitwarden" >&2
        return 1
      }
    fi
  fi

  echo "🔐 Unlocking Bitwarden vault..." >&2
  if [[ -n "${BW_PASSWORD:-}" ]]; then
    BW_SESSION=$(bw unlock --passwordenv BW_PASSWORD --raw) || {
      echo "❌ Failed to unlock Bitwarden vault" >&2
      return 1
    }
  else
    BW_SESSION=$(bw unlock --raw) || {
      echo "❌ Failed to unlock Bitwarden vault" >&2
      return 1
    }
  fi
  export BW_SESSION
}

# ---------------------------------------------------------------------------
# _resolve_bw_ref <item_name> <field_spec>
#   Resolves a bw:// reference. Prints the raw secret value to stdout.
#   field_spec: password | username | note | field:<name>
# ---------------------------------------------------------------------------
_resolve_bw_ref() {
  local item_name="$1"
  local field_spec="${2:-password}"

  _bw_ensure_session || return 1

  case "$field_spec" in
    password)
      bw get password "$item_name" --session "$BW_SESSION" 2>/dev/null
      ;;
    username)
      bw get username "$item_name" --session "$BW_SESSION" 2>/dev/null
      ;;
    note|notes)
      bw get notes "$item_name" --session "$BW_SESSION" 2>/dev/null
      ;;
    field:*)
      local fname="${field_spec#field:}"
      bw get item "$item_name" --session "$BW_SESSION" 2>/dev/null \
        | jq -re --arg f "$fname" '.fields[]? | select(.name == $f) | .value' \
        || { echo "❌ Field '$fname' not found in item '$item_name'" >&2; return 1; }
      ;;
    *)
      echo "❌ Unknown bw field spec '$field_spec'. Use: password, username, note, field:<name>" >&2
      return 1
      ;;
  esac
}

# ---------------------------------------------------------------------------
# _resolve_vault_ref_single <path> <field>
#   Resolves a single Vault KV field. Prints the raw value to stdout.
# ---------------------------------------------------------------------------
_resolve_vault_ref_single() {
  local vault_path="$1"
  local field="$2"

  if ! command -v vault >/dev/null 2>&1; then
    echo "❌ vault CLI not found. See: https://developer.hashicorp.com/vault/docs/install" >&2
    return 1
  fi

  : "${VAULT_ADDR:?VAULT_ADDR must be set for vault:// references}"

  vault kv get -field="$field" "$vault_path" 2>/dev/null || {
    echo "❌ Field '$field' not found at vault path '$vault_path'" >&2
    return 1
  }
}

# ---------------------------------------------------------------------------
# _resolve_env_file_nul <file>
#   Core resolver. Emits NUL-delimited KEY\0VALUE\0 pairs for every resolved
#   entry. NUL as delimiter ensures values containing spaces, newlines, or
#   any other special characters are handled correctly throughout.
#
#   For vault:// paths without a #field, emits one pair per KV field using
#   the vault field name (uppercased) as the key.
# ---------------------------------------------------------------------------
_resolve_env_file_nul() {
local env_file="${1:?Usage: _resolve_env_file_nul <file>}"

  if [[ ! -f "$env_file" ]]; then
    echo "❌ File not found: $env_file" >&2
    return 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip blank lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    # Skip lines without '='
    [[ "$line" != *=* ]] && continue

    local key="${line%%=*}"
    local value="${line#*=}"

    # Validate key: must be a legal shell variable name. Reject anything else
    # to prevent injection when the output is sourced (e.g. a key of "$(cmd)").
    if [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "❌ Invalid env var name '$key' in $env_file — skipping (keys must match [A-Za-z_][A-Za-z0-9_]*)" >&2
      continue
    fi

    # Strip surrounding quotes
    if [[ "$value" =~ ^\"(.*)\"$ ]]; then
	# BASH_REMATCH is an array variable that contains the results of the last regex match
	# performed by the =~ operator in bash. BASH_REMATCH[0] holds the entire matched string,
	# while BASH_REMATCH[1], BASH_REMATCH[2], etc. hold the captured groups (substrings in parentheses).
	# In this case, BASH_REMATCH[1] extracts the first captured group from the regex pattern match.
      value="${BASH_REMATCH[1]}"
    elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
      value="${BASH_REMATCH[1]}"
    fi

    if [[ "$value" == bw://* ]]; then
	# Removes the "bw://" prefix from the value variable and stores the result in ref
	# Uses bash parameter expansion to strip the prefix, leaving only the secret identifier
      local ref="${value#bw://}"
      local item_name field_spec

      if [[ "$ref" == */* ]]; then
        item_name="${ref%%/*}"
        field_spec="${ref#*/}"
      else
        item_name="$ref"
        field_spec="password"
      fi

      local resolved
      resolved=$(_resolve_bw_ref "$item_name" "$field_spec") || return 1
      printf '%s\0%s\0' "$key" "$resolved"

    elif [[ "$value" == vault://* ]]; then
      local ref="${value#vault://}"

      if [[ "$ref" == *#* ]]; then
        local vault_path="${ref%%#*}"
        local field="${ref#*#}"
        local resolved
        resolved=$(_resolve_vault_ref_single "$vault_path" "$field") || return 1
        printf '%s\0%s\0' "$key" "$resolved"
      else
        # All fields from vault path.
        # jq -j with \u0000 produces NUL-delimited KEY\0VALUE\0 pairs directly,
        # so values containing newlines or spaces are preserved exactly.
        if ! command -v vault >/dev/null 2>&1; then
          echo "❌ vault CLI not found." >&2; return 1
        fi
        : "${VAULT_ADDR:?VAULT_ADDR must be set for vault:// references}"
        vault kv get -format=json "$ref" 2>/dev/null \
          | jq -j '.data.data // .data | to_entries[] | "\(.key | ascii_upcase)\u0000\(.value)\u0000"' \
          || { echo "❌ Could not read vault path '$ref'" >&2; return 1; }
      fi

    else
      printf '%s\0%s\0' "$key" "$value"
    fi

  done < "$env_file"
}

# ---------------------------------------------------------------------------
# _load_self_env
#   Shell-portable self-loader. Detects bash or zsh, introspects the call
#   stack to find the .env file that sourced it, and calls resolve_env_file
#   on that file. If a previous env is active (_LOADED_ENV_FILE is set),
#   it is unloaded first so stale secrets never accumulate.
#
#   Used by the self-loading .env first-line pattern:
#     source <(curl -fsSL "https://.../resolve-env-refs.sh") \
#       && declare -f _load_self_env &>/dev/null \
#       && _load_self_env \
#       && return 0 2>/dev/null; true
# ---------------------------------------------------------------------------
_load_self_env() {
  local _env_file

  if [[ -n "${BASH_VERSION:-}" ]]; then
    # BASH_SOURCE[1] = the file that called this function
    _env_file="${BASH_SOURCE[1]}"
  elif [[ -n "${ZSH_VERSION:-}" ]]; then
    # funcfiletrace[1] = "filename:lineno" of the call site; strip the line number
    local _ft="${funcfiletrace[1]}"
    _env_file="${_ft%:*}"
  else
    echo "❌ resolve-env-refs: unsupported shell (bash or zsh required)" >&2
    return 1
  fi

  if [[ -z "${_env_file:-}" || ! -f "${_env_file}" ]]; then
    echo "❌ resolve-env-refs: cannot determine caller file; use resolve_env_file <path> instead" >&2
    return 1
  fi

  # Unload any previously active env before loading the new one.
  # Prints a message when switching projects so the change is visible.
  if [[ -n "${_LOADED_ENV_FILE:-}" ]]; then
    if [[ "${_LOADED_ENV_FILE}" != "${_env_file}" ]]; then
      echo "🔄 Switching env: ${_LOADED_ENV_FILE} → ${_env_file}" >&2
    fi
    { type unload_env &>/dev/null && unload_env; } 2>/dev/null || true
  fi

  source <(resolve_env_file "${_env_file}")
}

# ---------------------------------------------------------------------------
# resolve_env_file <file>
#   Public API for source mode. Reads the .env file, resolves references,
#   and prints shell-escaped "export KEY=VALUE" lines safe for:
#     source <(./snippets/resolve-env-refs.sh .env.example)
#
#   When sourced outside of direnv, also emits:
#     - _LOADED_ENV_FILE  — tracks which file is currently active
#     - unload_env()      — idempotent cleanup function
#     - trap unload_env EXIT — auto-cleanup when the shell exits
#
#   Inside direnv, no tracking or trap is emitted — direnv manages its own
#   env diff and reverts changes automatically when you leave the directory.
# ---------------------------------------------------------------------------
resolve_env_file() {
  local _file="${1:?Usage: resolve_env_file <file>}"
  local _k _v
  local -a _keys=()

  while IFS= read -r -d $'\0' _k && IFS= read -r -d $'\0' _v; do
    # Validate key at point of emission — catches Vault-derived keys that bypass
    # the .env-line validation (e.g. a Vault field named "$(cmd)").
    if [[ ! "$_k" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "❌ Invalid key '$_k' (from resolved source) — skipping" >&2
      continue
    fi
    printf 'export %s=%q\n' "$_k" "$_v"
    _keys+=("$_k")
  done < <(_resolve_env_file_nul "$_file")

  # Emit cleanup — skip inside direnv (it manages its own env diff and revert)
  if [[ ${#_keys[@]} -gt 0 ]] && [[ -z "${DIRENV_DIR:-}" ]]; then
    printf '_LOADED_ENV_VARS="${_LOADED_ENV_VARS:+${_LOADED_ENV_VARS} }%s"\n' "${_keys[*]}"
    printf '_LOADED_ENV_FILE=%q\n' "$_file"
    printf 'unload_env() { [[ -n "${_LOADED_ENV_VARS:-}" ]] && unset ${_LOADED_ENV_VARS} 2>/dev/null; unset _LOADED_ENV_VARS _LOADED_ENV_FILE 2>/dev/null; }\n'
    printf 'trap unload_env EXIT\n'
  fi
}

# ---------------------------------------------------------------------------
# Main
#   With "-- command": resolve and exec the command with vars injected.
#   Without "--": emit shell-escaped exports for "source <(...)".
# ---------------------------------------------------------------------------
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
  if [[ $# -eq 0 ]]; then
    cat >&2 <<'EOF'
Usage:
  # Mode 1 — exec (recommended, safe for any secret value):
  resolve-env-refs.sh <env-file> -- <command> [args...]

  # Mode 2 — source into current shell:
  source <(resolve-env-refs.sh <env-file>)

  ⚠️  Never use eval — use "source <(...)".

Reference formats in your .env.example:
  DATABASE_PASSWORD=bw://prod-db/password
  DATABASE_USER=bw://prod-db/username
  STRIPE_KEY=bw://stripe-api/field:api_key
  VAULT_SECRET=vault://secret/myproject/app#api_key
  VAULT_ALL=vault://secret/myproject/app
EOF
    exit 1
  fi

  env_file="$1"
# Removes the first positional parameter ($1) from the list of arguments,
# shifting all remaining parameters down by one position ($2 becomes $1, etc.).
# This is typically used to process arguments sequentially in a loop.
  shift

  if [[ "${1:-}" == "--" ]]; then
    shift
    if [[ $# -eq 0 ]]; then
      echo "❌ No command specified after '--'" >&2
      exit 1
    fi

    # Build env array directly from NUL-delimited pairs — no re-parsing, no eval.
    # Values are kept byte-for-byte as emitted by the resolvers.
    # Key validation here is defence-in-depth (exec mode is not injection-prone,
    # but we reject garbage keys to avoid passing nonsense to the child process).
    declare -a env_vars=()
    while IFS= read -r -d $'\0' k && IFS= read -r -d $'\0' v; do
      if [[ ! "$k" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
        echo "❌ Invalid key '$k' (from resolved source) — skipping" >&2
        continue
      fi
      env_vars+=("${k}=${v}")
    done < <(_resolve_env_file_nul "$env_file")

    exec env "${env_vars[@]}" "$@"
  else
    resolve_env_file "$env_file"
  fi
fi
