# Bitwarden CLI — Scripting Guide

Using Bitwarden in automated scripts requires careful handling of the session key.

---

## Session Key Management

The session key is a short-lived bearer token for your vault. Rules:

1. **Never hardcode it** — obtain it at runtime
2. **Never log it** — redirect output appropriately
3. **Set a shell timeout** — lock vault when script completes
4. **Use `--raw`** — avoids extra output that could leak into logs

---

## Pattern 1: Source a Helper Function

Add to `~/.bashrc` or `~/.zshrc`:

```bash
bw_session() {
  if [[ -z "$BW_SESSION" ]]; then
    export BW_SESSION=$(bw unlock --raw 2>/dev/null) || {
      echo "Failed to unlock Bitwarden vault" >&2
      return 1
    }
  fi
}

bw_get() {
  local item_name="$1"
  local field="${2:-password}"
  bw_session || return 1
  bw get "$field" "$item_name" --session "$BW_SESSION" 2>/dev/null
}
```

Usage in scripts:

```bash
source ~/.bashrc
DB_PASSWORD=$(bw_get "prod-db" password)
API_KEY=$(bw_get "stripe-api" | jq -r '.fields[] | select(.name=="api_key") | .value')
```

---

## Pattern 2: Unattended Script with API Key Auth

For CI/CD or cron jobs where interactive unlock is not possible:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Credentials should be set as environment secrets (CI/CD secrets manager)
: "${BW_CLIENTID:?BW_CLIENTID must be set}"
: "${BW_CLIENTSECRET:?BW_CLIENTSECRET must be set}"
: "${BW_PASSWORD:?BW_PASSWORD must be set}"

# Login with API key (idempotent — safe to run repeatedly)
bw login --apikey 2>/dev/null || true

# Unlock vault
BW_SESSION=$(bw unlock --passwordenv BW_PASSWORD --raw)
export BW_SESSION

# Sync latest
bw sync --session "$BW_SESSION" >/dev/null

# Retrieve secret
DB_PASSWORD=$(bw get password "prod-db" --session "$BW_SESSION")

# Use secret...
echo "Connecting to database..."

# Lock vault when done
bw lock >/dev/null
```

---

## Pattern 3: Inject into a Child Process

Retrieve secrets from Bitwarden and inject into a child process **without writing to disk or shell history**:

```bash
#!/usr/bin/env bash
set -euo pipefail

BW_SESSION=$(bw unlock --raw)
export BW_SESSION

DB_HOST=$(bw get item "prod-db" --session "$BW_SESSION" | jq -r '.fields[] | select(.name=="host") | .value')
DB_USER=$(bw get item "prod-db" --session "$BW_SESSION" | jq -r '.login.username')
DB_PASS=$(bw get password "prod-db" --session "$BW_SESSION")

# Export for child process, then unset immediately after
DB_HOST="$DB_HOST" DB_USER="$DB_USER" DB_PASS="$DB_PASS" ./my-app

bw lock >/dev/null
```

---

## Handling Errors

```bash
# Check bw is logged in before proceeding
if ! bw status --session "$BW_SESSION" | grep -q '"status":"unlocked"'; then
  echo "Bitwarden vault is not unlocked" >&2
  exit 1
fi

# Validate a secret was retrieved (non-empty)
SECRET=$(bw get password "my-service" --session "$BW_SESSION")
if [[ -z "$SECRET" ]]; then
  echo "Secret not found in Bitwarden" >&2
  exit 1
fi
```

---

## Security Checklist for Scripts

- [ ] Script does not log or `echo` secrets
- [ ] `BW_SESSION` is not passed as a command-line argument (use `export`)
- [ ] Script calls `bw lock` in a `trap` or finally block
- [ ] `set -euo pipefail` is at the top of the script
- [ ] The script file itself has `chmod 700` permissions
- [ ] CI/CD secrets (API key, password) are stored in the CI platform's secret manager, not in the repo

Add a lock trap:

```bash
trap 'bw lock >/dev/null 2>&1' EXIT
```
