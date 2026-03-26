# Secret Injector Agent

An AI agent that injects secrets from Vault or Bitwarden into the environment of a child process — without persisting secrets to disk or shell history.

---

## What It Does

- Retrieves secrets from HashiCorp Vault (KV v2) or Bitwarden CLI
- Sets them as environment variables scoped to a single child process
- Cleans up on exit — secrets do not persist in the parent shell
- Supports interactive and unattended (CI/CD) modes

---

## Prerequisites

**For Vault backend:**
- `vault` CLI installed
- `VAULT_ADDR` set
- Valid `VAULT_TOKEN` or authenticated via `vault login`

**For Bitwarden backend:**
- `bw` CLI installed
- `BW_SESSION` set (`export BW_SESSION=$(bw unlock --raw)`)

---

## Usage

```bash
# Inject from Vault and run a command
./inject.sh vault secret/myproject/dev -- node server.js

# Inject from Bitwarden and run a command
./inject.sh bitwarden "my-service" -- python app.py

# Inject from multiple Vault paths
./inject.sh vault secret/myproject/dev secret/myproject/prod-overrides -- ./start.sh

# Dry run — print what would be exported (values masked)
./inject.sh vault secret/myproject/dev --dry-run -- node server.js
```

---

## Environment Variable Naming

Vault field names and Bitwarden field names are **uppercased** when exported:

| Source field | Exported as |
|---|---|
| `database_url` | `DATABASE_URL` |
| `api_key` | `API_KEY` |
| `username` (Bitwarden login) | `USERNAME` |
| `password` (Bitwarden login) | `PASSWORD` |

---

## Security Model

- Secrets are injected as environment variables only for the duration of the child process
- The parent shell's environment is never modified
- No secrets are written to disk, `/tmp`, or shell history
- If secret retrieval fails, the agent exits non-zero — the child process is **not** started with empty/missing credentials

---

## CI/CD Integration

```yaml
# GitHub Actions example
- name: Run app with injected secrets
  env:
    VAULT_ADDR: ${{ vars.VAULT_ADDR }}
    VAULT_TOKEN: ${{ secrets.VAULT_TOKEN }}
  run: |
    ./agents/secret-injector/inject.sh vault secret/myproject/prod -- ./start.sh
```
