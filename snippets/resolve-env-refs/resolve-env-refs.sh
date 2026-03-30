#!/usr/bin/env bash
# resolve-env-refs.sh — Resolve bw:// and vault:// references in .env and YAML files.
#
# Install: ./snippets/resolve-env-refs/install.sh
# Docs:    ./snippets/resolve-env-refs/README.md
#
# ─────────────────────────────────────────────────────────────────────────────
# Reference formats
# ─────────────────────────────────────────────────────────────────────────────
#
#   bw://item-name              → Bitwarden password field (default)
#   bw://item-name/password     → Bitwarden password field
#   bw://item-name/username     → Bitwarden username field
#   bw://item-name/note         → Bitwarden notes field
#   bw://item-name/field:fname  → Bitwarden custom field named "fname"
#   vault://secret/path#field   → Vault KV v2 single field
#   vault://secret/path         → Vault KV v2 all fields (.env mode only)
#
# ─────────────────────────────────────────────────────────────────────────────
# .env / .envrc usage
# ─────────────────────────────────────────────────────────────────────────────
#
#   First, make the functions available:
#     source ~/.config/resolve-env-refs/resolve-env-refs.sh
#
#   Pattern 1 — direnv (auto-loads and unloads on directory change):
#     # .envrc
#     source ~/.config/resolve-env-refs/resolve-env-refs.sh
#     source <(resolve_env_file .env)
#     # Then: direnv allow .
#
#   Pattern 2 — self-loading .env (bash and zsh):
#     # .env — first line:
#     source ~/.config/resolve-env-refs/resolve-env-refs.sh \
#       && declare -f _load_self_env &>/dev/null \
#       && _load_self_env \
#       && return 0 2>/dev/null; true
#
#     DATABASE_URL=bw://prod-db/password
#     STRIPE_KEY=vault://secret/stripe#api_key
#
#     Then: source .env        # resolves refs, sets EXIT cleanup
#           unload_env         # manual cleanup
#
#   ⚠️  The EXIT trap from resolve_env_file replaces any pre-existing EXIT trap.
#       To chain: trap 'unload_env; your_existing_cleanup' EXIT
#
#   Pattern 3 — exec mode (secrets never enter your shell):
#     resolve-env-refs.sh .env -- node server.js
#     source <(resolve-env-refs.sh .env)
#
#   ⚠️  Never use: eval "$(resolve-env-refs.sh .env)" — use source <(...) instead.
#
# ─────────────────────────────────────────────────────────────────────────────
# YAML usage
# ─────────────────────────────────────────────────────────────────────────────
#
#   Resolves bw:// and vault:// values in YAML files. Output goes to stdout
#   so secrets are never written to disk in stream mode.
#
#   Stream mode (pipe resolved YAML directly to a tool):
#     resolve_yaml_file values.yaml | helm upgrade myapp . -f -
#     resolve_yaml_file config.yaml | kubectl apply -f -
#
#   Exec mode (for tools requiring a file path; {} is replaced by temp file):
#     resolve_yaml_exec values.yaml -- helm upgrade myapp . -f {}
#     resolve_yaml_exec config.yaml -- kubectl apply -f {}
#
#   Temp file is created in /dev/shm (RAM) when available, chmod 600,
#   and deleted immediately after the command exits.
#
#   Single value extraction (for bash scripts):
#     DB_PASS=$(resolve_yaml_value config.yaml database.password)
#     API_KEY=$(resolve_yaml_value config.yaml api.stripe_key)
#     Requires yq or python3+pyyaml.
#
#   ⚠️  vault:// without #field is not supported in YAML mode.
#       Use vault://secret/path#fieldname to target a single field.
#
#   Python drop-in: see resolve_yaml_refs.py in the same directory.
#
# ─────────────────────────────────────────────────────────────────────────────
# Prerequisites
# ─────────────────────────────────────────────────────────────────────────────
#
#   bw://  references: bw CLI installed; BW_SESSION must be set (export BW_SESSION=$(bw unlock --raw))
#   vault:// references: vault CLI; VAULT_ADDR and VAULT_TOKEN set

# Strict mode only when running as a script. When sourced, functions handle
# errors explicitly with '|| return 1'. Sourcing must not alter caller's options.
[[ "${BASH_SOURCE[0]:-}" != "${0}" ]] || set -euo pipefail

# ---------------------------------------------------------------------------
# _bw_ensure_session
#   Requires BW_SESSION to already be set. Fails with a clear message if not.
#   To set: export BW_SESSION=$(bw unlock --raw)
# ---------------------------------------------------------------------------
_bw_ensure_session() {
  if [[ -z "${BW_SESSION:-}" ]]; then
    echo "❌ BW_SESSION is not set." >&2
    echo "   Run: export BW_SESSION=\$(bw unlock --raw)" >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# _bw_get <bw args...>
#   Thin wrapper around the bw CLI. Passes stdout through unchanged. On
#   failure, surfaces bw's own stderr (which 2>/dev/null would have hidden)
#   before returning 1, so callers can print a context-specific message.
# ---------------------------------------------------------------------------
_bw_get() {
  local _bw_stderr _rc=0
  _bw_stderr=$(mktemp)
  bw "$@" 2>"$_bw_stderr" || _rc=$?
  if [[ $_rc -ne 0 ]]; then
    cat "$_bw_stderr" >&2
    rm -f "$_bw_stderr"
    return 1
  fi
  rm -f "$_bw_stderr"
}

# ---------------------------------------------------------------------------
# _bw_get_item_json <item_name>
#   Retrieves a single Bitwarden item as JSON using 'bw list items --search',
#   then filters to an exact name match. This avoids the 'bw get' code path
#   that crashes with a null-pointer on org items when org keys aren't loaded
#   (known Bitwarden CLI bug: orgKeys is null after bw unlock --raw).
#
#   Fails if the name matches zero items ("not found") or more than one item
#   ("ambiguous") to prevent silently injecting credentials from the wrong item.
# ---------------------------------------------------------------------------
_bw_get_item_json() {
  local item_name="$1"

  if ! command -v jq >/dev/null 2>&1; then
    echo "❌ jq is required for Bitwarden lookups. Install: https://stedolan.github.io/jq/" >&2
    return 1
  fi

  local _list_json
  _list_json=$(_bw_get list items --search "$item_name" --session "$BW_SESSION") || {
    echo "❌ Failed to list Bitwarden items (check BW_SESSION is valid)" >&2
    return 1
  }

  local _count
  _count=$(printf '%s' "$_list_json" \
    | jq --arg n "$item_name" '[.[] | select(.name == $n)] | length') || {
    echo "❌ jq parse error while searching for '$item_name'" >&2
    return 1
  }

  case "$_count" in
    0)
      echo "❌ Bitwarden item '$item_name' not found" >&2
      return 1
      ;;
    1) ;;
    *)
      echo "❌ Bitwarden item name '$item_name' is ambiguous — $_count items match. Rename them to be unique." >&2
      return 1
      ;;
  esac

  printf '%s' "$_list_json" \
    | jq -e --arg n "$item_name" 'first(.[] | select(.name == $n))'
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

  local _item_json
  _item_json=$(_bw_get_item_json "$item_name") || return 1

  case "$field_spec" in
    password)
      printf '%s' "$_item_json" | jq -re '.login.password // empty' || {
        echo "❌ Bitwarden item '$item_name': no password field" >&2
        return 1
      }
      ;;
    username)
      printf '%s' "$_item_json" | jq -re '.login.username // empty' || {
        echo "❌ Bitwarden item '$item_name': no username field" >&2
        return 1
      }
      ;;
    note|notes)
      printf '%s' "$_item_json" | jq -re '.notes // empty' || {
        echo "❌ Bitwarden item '$item_name': no notes field" >&2
        return 1
      }
      ;;
    field:*)
      local fname="${field_spec#field:}"
      printf '%s' "$_item_json" \
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
#   Core resolver for .env files. Emits NUL-delimited KEY\0VALUE\0 pairs.
#   NUL delimiter ensures values with spaces, newlines, or special characters
#   are handled correctly throughout the pipeline.
#
#   vault:// paths without #field emit one pair per KV field, using the
#   Vault field name (uppercased) as the key.
# ---------------------------------------------------------------------------
_resolve_env_file_nul() {
  local env_file="${1:?Usage: _resolve_env_file_nul <file>}"

  if [[ ! -f "$env_file" ]]; then
    echo "❌ File not found: $env_file" >&2
    return 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    [[ "$line" != *=* ]] && continue

    local key="${line%%=*}"
    local value="${line#*=}"

    # Validate key: must be a legal shell variable name to prevent injection
    if [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "❌ Invalid env var name '$key' in $env_file — skipping (keys must match [A-Za-z_][A-Za-z0-9_]*)" >&2
      continue
    fi

    # Strip surrounding quotes
    if [[ "$value" =~ ^\"(.*)\"$ ]]; then
      value="${BASH_REMATCH[1]}"
    elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
      value="${BASH_REMATCH[1]}"
    fi

    if [[ "$value" == bw://* ]]; then
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
        # All fields from vault path — NUL-delimited KEY\0VALUE\0 pairs
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
#   Shell-portable self-loader. Introspects the call stack to find the .env
#   file that sourced it, then calls resolve_env_file on that file.
#   If a previous env is active, it is unloaded first.
#
#   Used by the self-loading .env first-line pattern:
#     source ~/.config/resolve-env-refs/resolve-env-refs.sh \
#       && declare -f _load_self_env &>/dev/null \
#       && _load_self_env \
#       && return 0 2>/dev/null; true
# ---------------------------------------------------------------------------
_load_self_env() {
  local _env_file

  if [[ -n "${BASH_VERSION:-}" ]]; then
    _env_file="${BASH_SOURCE[1]}"
  elif [[ -n "${ZSH_VERSION:-}" ]]; then
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
#   Public API. Reads the .env file, resolves references, and prints
#   shell-escaped "export KEY=VALUE" lines safe for:
#     source <(resolve_env_file .env)
#
#   Outside direnv, also emits:
#     - _LOADED_ENV_FILE  — tracks which file is currently active
#     - unload_env()      — idempotent cleanup function
#     - trap unload_env EXIT — auto-cleanup when the shell exits
#
#   Inside direnv, no tracking or trap is emitted — direnv manages its own
#   env diff and revert automatically.
#
#   ⚠️  trap unload_env EXIT replaces any pre-existing EXIT trap.
#       To chain: trap 'unload_env; your_cleanup' EXIT
# ---------------------------------------------------------------------------
resolve_env_file() {
  local _file="${1:?Usage: resolve_env_file <file>}"
  local _k _v
  local -a _keys=()

  # Run resolver to a secure temp buffer first so any failure is observable
  # before we emit any output. /dev/shm is RAM-backed; fall back to mktemp.
  local _tmpbuf
  if [[ -d /dev/shm ]]; then
    _tmpbuf=$(mktemp /dev/shm/resolve-env-XXXXXX)
  else
    _tmpbuf=$(mktemp -t resolve-env-XXXXXX)
  fi
  chmod 600 "$_tmpbuf"

  if ! _resolve_env_file_nul "$_file" > "$_tmpbuf"; then
    rm -f "$_tmpbuf"
    return 1
  fi

  while IFS= read -r -d $'\0' _k && IFS= read -r -d $'\0' _v; do
    # Validate Vault-derived keys that bypass .env-line validation
    if [[ ! "$_k" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "❌ Invalid key '$_k' (from resolved source) — skipping" >&2
      continue
    fi
    printf 'export %s=%q\n' "$_k" "$_v"
    _keys+=("$_k")
  done < "$_tmpbuf"
  rm -f "$_tmpbuf"

  if [[ ${#_keys[@]} -gt 0 ]] && [[ -z "${DIRENV_DIR:-}" ]]; then
    printf '_LOADED_ENV_VARS="${_LOADED_ENV_VARS:+${_LOADED_ENV_VARS} }%s"\n' "${_keys[*]}"
    printf '_LOADED_ENV_FILE=%q\n' "$_file"
    printf 'unload_env() { [[ -n "${_LOADED_ENV_VARS:-}" ]] && unset ${_LOADED_ENV_VARS} 2>/dev/null; unset _LOADED_ENV_VARS _LOADED_ENV_FILE 2>/dev/null; }\n'
    printf 'trap unload_env EXIT\n'
  fi
}

# ---------------------------------------------------------------------------
# _yaml_dquote <value>
#   Emits value as a YAML double-quoted scalar, escaping all characters that
#   require escaping inside YAML double-quoted strings.
# ---------------------------------------------------------------------------
_yaml_dquote() {
  local v="$1"
  v="${v//\\/\\\\}"        # backslash first
  v="${v//\"/\\\"}"        # double-quote
  v="${v//$'\n'/\\n}"      # newline
  v="${v//$'\r'/\\r}"      # carriage return
  v="${v//$'\t'/\\t}"      # tab
  printf '"%s"' "$v"
}

# ---------------------------------------------------------------------------
# resolve_yaml_file <file>
#   Resolves bw:// and vault://#field references in YAML scalar values.
#   Outputs clean YAML to stdout — secrets are never written to disk.
#
#   Handles unquoted, single-quoted, and double-quoted scalar values.
#   Resolved values are always emitted as double-quoted YAML strings.
#   Lines without references are passed through unchanged.
#
#   ⚠️  vault:// without #field is not supported (multi-field expansion
#       cannot map to a single YAML scalar). Use vault://path#field.
#
#   Usage:
#     resolve_yaml_file values.yaml | helm upgrade myapp . -f -
#     resolve_yaml_file config.yaml | kubectl apply -f -
# ---------------------------------------------------------------------------
resolve_yaml_file() {
  local yaml_file="${1:?Usage: resolve_yaml_file <file>}"

  if [[ ! -f "$yaml_file" ]]; then
    echo "❌ File not found: $yaml_file" >&2
    return 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    # Fast path: skip lines with no references
    if [[ "$line" != *"bw://"* && "$line" != *"vault://"* ]]; then
      printf '%s\n' "$line"
      continue
    fi

    # Skip comment lines
    if [[ "$line" =~ ^[[:space:]]*# ]]; then
      printf '%s\n' "$line"
      continue
    fi

    # Must be a key: value line
    if [[ "$line" != *:* ]]; then
      printf '%s\n' "$line"
      continue
    fi

    local key_raw="${line%%:*}"
    local after_colon="${line#*:}"

    # Capture leading whitespace between colon and value
    local val_lws=""
    if [[ "$after_colon" =~ ^([[:space:]]+)(.*) ]]; then
      val_lws="${BASH_REMATCH[1]}"
      after_colon="${BASH_REMATCH[2]}"
    fi

    # Match: optional-quote ref optional-quote optional-trailing-comment
    # Quoted values: # inside quotes is not a comment — match the full quoted string.
    # Unquoted bw://: # is NOT part of the ref; a trailing "# comment" is allowed.
    # Unquoted vault://: # IS part of the ref (path#field); only SPACE+# starts a comment.
    local ref_val="" trailing=""
    if [[ "$after_colon" =~ ^\"(bw://[^\"[:space:]]+|vault://[^\"[:space:]]+)\"([[:space:]]*#.*)?$ ]]; then
      ref_val="${BASH_REMATCH[1]}"
      trailing="${BASH_REMATCH[2]:-}"
    elif [[ "$after_colon" =~ ^\'(bw://[^\'[:space:]]+|vault://[^\'[:space:]]+)\'([[:space:]]*#.*)?$ ]]; then
      ref_val="${BASH_REMATCH[1]}"
      trailing="${BASH_REMATCH[2]:-}"
    elif [[ "$after_colon" =~ ^(bw://[^[:space:]\"\'#]+)([[:space:]]*#.*)?$ ]]; then
      ref_val="${BASH_REMATCH[1]}"
      trailing="${BASH_REMATCH[2]:-}"
    elif [[ "$after_colon" =~ ^(vault://[^[:space:]\"\']+)([[:space:]]+#.*)?$ ]]; then
      # vault:// refs include #field — only whitespace-prefixed # starts a comment
      ref_val="${BASH_REMATCH[1]}"
      trailing="${BASH_REMATCH[2]:-}"
    else
      # Pattern not recognised — pass through unchanged
      printf '%s\n' "$line"
      continue
    fi

    # Resolve the reference
    local resolved
    if [[ "$ref_val" == bw://* ]]; then
      local bw_ref="${ref_val#bw://}"
      local item_name field_spec
      if [[ "$bw_ref" == */* ]]; then
        item_name="${bw_ref%%/*}"
        field_spec="${bw_ref#*/}"
      else
        item_name="$bw_ref"
        field_spec="password"
      fi
      resolved=$(_resolve_bw_ref "$item_name" "$field_spec") || return 1

    else
      local vault_ref="${ref_val#vault://}"
      if [[ "$vault_ref" != *#* ]]; then
        echo "❌ vault:// without #field is not supported in YAML mode (multi-field expansion cannot be a single scalar). Use vault://path#field." >&2
        return 1
      fi
      local vault_path="${vault_ref%%#*}"
      local field="${vault_ref#*#}"
      resolved=$(_resolve_vault_ref_single "$vault_path" "$field") || return 1
    fi

    # Emit line with resolved value always double-quoted for safety
    printf '%s:%s%s%s\n' "$key_raw" "$val_lws" "$(_yaml_dquote "$resolved")" "$trailing"

  done < "$yaml_file"
}

# ---------------------------------------------------------------------------
# resolve_yaml_exec <file> -- <command> [args...]
#   Resolves a YAML file and writes the result to a secure temp file
#   (/dev/shm when available, chmod 600), then runs the command with {}
#   replaced by the temp file path in any argument. The temp file is
#   deleted immediately after the command exits.
#
#   Usage:
#     resolve_yaml_exec values.yaml -- helm upgrade myapp . -f {}
#     resolve_yaml_exec config.yaml -- kubectl apply -f {}
# ---------------------------------------------------------------------------
resolve_yaml_exec() {
  local yaml_file="${1:?Usage: resolve_yaml_exec <file> -- <command> [args...]}"
  shift

  [[ "${1:-}" == "--" ]] && shift
  if [[ $# -eq 0 ]]; then
    echo "❌ No command specified after '--'" >&2
    return 1
  fi

  # Prefer /dev/shm (RAM-backed) to keep secrets off disk
  local tmpfile
  if [[ -d /dev/shm ]]; then
    tmpfile=$(mktemp /dev/shm/resolve-yaml-XXXXXX.yaml)
  else
    tmpfile=$(mktemp -t resolve-yaml-XXXXXX.yaml)
  fi
  chmod 600 "$tmpfile"

  resolve_yaml_file "$yaml_file" > "$tmpfile" || { rm -f "$tmpfile"; return 1; }

  # Replace {} with the temp file path in all arguments
  local -a cmd_args=()
  for arg in "$@"; do
    cmd_args+=("${arg//\{\}/$tmpfile}")
  done

  # Run in subshell so the EXIT trap cleans up even on signals
  ( trap "rm -f '$tmpfile'" EXIT; "${cmd_args[@]}"; )
  local rc=$?
  rm -f "$tmpfile" 2>/dev/null || true
  return $rc
}

# ---------------------------------------------------------------------------
# resolve_yaml_value <file> <key.path>
#   Extracts a single resolved value from a YAML file using dot notation.
#   Resolves any bw:// or vault:// reference in the target value.
#
#   key.path uses dots to traverse nested keys. List indices are integers.
#   Examples:
#     database.password
#     api.keys.stripe
#     services.0.url
#
#   Requires yq (https://github.com/mikefarah/yq) or python3 with pyyaml.
#
#   Usage:
#     DB_PASS=$(resolve_yaml_value config.yaml database.password)
#     API_KEY=$(resolve_yaml_value config.yaml api.stripe_key)
# ---------------------------------------------------------------------------
resolve_yaml_value() {
  local yaml_file="${1:?Usage: resolve_yaml_value <file> <key.path>}"
  local key_path="${2:?Usage: resolve_yaml_value <file> <key.path>}"

  if command -v yq >/dev/null 2>&1; then
    resolve_yaml_file "$yaml_file" | yq ".$key_path"
  elif command -v python3 >/dev/null 2>&1; then
    resolve_yaml_file "$yaml_file" | \
      python3 -c "
import sys, yaml
data = yaml.safe_load(sys.stdin)
v = data
for part in sys.argv[1].split('.'):
    v = v[part] if isinstance(v, dict) else v[int(part)]
print(v, end='')
" "$key_path"
  else
    echo "❌ resolve_yaml_value requires yq or python3 with pyyaml" >&2
    return 1
  fi
}


#   With "-- command": resolve .env and exec the command with vars injected.
#   Without "--":      emit shell-escaped exports for "source <(...)".
#   With "--yaml":     resolve YAML file to stdout, or exec with temp file.
# ---------------------------------------------------------------------------
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]]; then
  if [[ $# -eq 0 ]]; then
    cat >&2 <<'EOF'
Usage:
  # .env exec mode — resolved vars injected into child process only:
  resolve-env-refs.sh <env-file> -- <command> [args...]

  # .env source mode — emit shell-escaped exports:
  source <(resolve-env-refs.sh <env-file>)

  # YAML stream mode — resolved YAML to stdout:
  resolve-env-refs.sh --yaml <yaml-file>

  # YAML exec mode — {} is replaced with a secure temp file path:
  resolve-env-refs.sh --yaml <yaml-file> -- <command> [args...]

  ⚠️  Never use: eval "$(resolve-env-refs.sh <env-file>)"
      Use "source <(...)" or exec mode instead.

Reference formats:
  DATABASE_PASSWORD=bw://prod-db/password
  DATABASE_USER=bw://prod-db/username
  STRIPE_KEY=bw://stripe-api/field:api_key
  VAULT_SECRET=vault://secret/myproject/app#api_key
  VAULT_ALL=vault://secret/myproject/app        # .env only
EOF
    exit 1
  fi

  if [[ "${1:-}" == "--yaml" ]]; then
    shift
    yaml_file="${1:?--yaml requires a file argument}"
    shift

    if [[ "${1:-}" == "--" ]]; then
      shift
      if [[ $# -eq 0 ]]; then
        echo "❌ No command specified after '--'" >&2
        exit 1
      fi
      resolve_yaml_exec "$yaml_file" -- "$@"
    else
      resolve_yaml_file "$yaml_file"
    fi
    exit $?
  fi

  env_file="$1"
  shift

  if [[ "${1:-}" == "--" ]]; then
    shift
    if [[ $# -eq 0 ]]; then
      echo "❌ No command specified after '--'" >&2
      exit 1
    fi

    declare -a env_vars=()
    local _exec_tmpbuf
    if [[ -d /dev/shm ]]; then
      _exec_tmpbuf=$(mktemp /dev/shm/resolve-env-XXXXXX)
    else
      _exec_tmpbuf=$(mktemp -t resolve-env-XXXXXX)
    fi
    chmod 600 "$_exec_tmpbuf"

    if ! _resolve_env_file_nul "$env_file" > "$_exec_tmpbuf"; then
      rm -f "$_exec_tmpbuf"
      exit 1
    fi

    while IFS= read -r -d $'\0' k && IFS= read -r -d $'\0' v; do
      if [[ ! "$k" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
        echo "❌ Invalid key '$k' (from resolved source) — skipping" >&2
        continue
      fi
      env_vars+=("${k}=${v}")
    done < "$_exec_tmpbuf"
    rm -f "$_exec_tmpbuf"

    exec env "${env_vars[@]}" "$@"
  else
    resolve_env_file "$env_file"
  fi
fi
