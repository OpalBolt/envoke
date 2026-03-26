# Snippets

Snippets are ready-to-source shell functions for common secret-management tasks. Copy them into your project, `source` them in your shell profile, or use them directly in scripts.

They are deliberately minimal — single-purpose functions with no external dependencies beyond the tool they wrap (Vault CLI, Bitwarden CLI, git, kubectl).

---

## When to use snippets vs examples

| Use snippets when… | Use [examples](../examples/) when… |
|--------------------|-------------------------------------|
| You need shell-level secret retrieval | You're integrating secrets into application code |
| You want reusable functions in `.bashrc` / `.zshrc` | You need a complete, runnable client in Python / Go / TypeScript |
| You're writing a Bash deploy or ops script | You're building a service that reads secrets at startup |
| You want a pre-commit hook or kubeconfig helper | You need typed interfaces and proper error handling in a language |

---

## How to use a snippet

### Option 1 — Source in your shell profile (recommended for daily use)

```bash
# Add to ~/.bashrc or ~/.zshrc
source ~/path/to/snippets/vault-login.sh
source ~/path/to/snippets/bw-get-secret.sh
```

Functions are then available in every new shell session.

### Option 2 — Source at the top of a script

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../snippets/vault-login.sh"

vault_oidc_login
```

### Option 3 — Copy the function directly into your script

If you don't want a dependency on the snippets directory, paste the function body directly. Each snippet is written to be self-contained.

---

## Snippets reference

### [`vault-login.sh`](vault-login.sh)

Vault authentication helpers. Wraps the Vault CLI auth methods described in [guides/vault/authentication.md](../guides/vault/authentication.md).

**Prerequisites:** `vault` CLI, `VAULT_ADDR` set.

**Functions:**

| Function | Description |
|----------|-------------|
| `vault_login_oidc` | Interactive OIDC/SSO login (opens browser) |
| `vault_login_approle` | AppRole login for scripts/CI |
| `vault_login_token` | Login with an existing token |
| `vault_token_info` | Print current token details |
| `vault_token_renew` | Renew the current token TTL |
| `vault_logout` | Revoke token and unset `VAULT_TOKEN` |

**Usage:**

```bash
source snippets/vault-login.sh

# Interactive login (humans)
vault_login_oidc

# AppRole login (scripts / CI)
ROLE_ID="<role-id>" SECRET_ID="<secret-id>" vault_login_approle

# Renew before TTL expires
vault_token_renew

# Clean up
vault_logout
```

**Related:** [Vault authentication guide](../guides/vault/authentication.md)

---

### [`bw-get-secret.sh`](bw-get-secret.sh)

Bitwarden secret retrieval helpers. Wraps `bw` CLI commands described in [guides/bitwarden/usage.md](../guides/bitwarden/usage.md).

**Prerequisites:** `bw` CLI, vault unlocked (`BW_SESSION` set or prompted).

**Functions:**

| Function | Description |
|----------|-------------|
| `bw_ensure_unlocked` | Unlock vault if `BW_SESSION` is not set |
| `bw_get_password` | Get password for a named item |
| `bw_get_username` | Get username for a named item |
| `bw_get_field` | Get a custom field value by name |
| `bw_get_note` | Get secure note contents |
| `bw_get_totp` | Get TOTP code for a named item |
| `bw_lock` | Lock the vault and unset `BW_SESSION` |

**Usage:**

```bash
source snippets/bw-get-secret.sh

DB_PASS=$(bw_get_password "prod-db")
API_KEY=$(bw_get_field "stripe" "api_key")
SSH_KEY=$(bw_get_note "server-ssh-key")

bw_lock
```

**Related:** [Bitwarden usage guide](../guides/bitwarden/usage.md) · [Bitwarden scripting guide](../guides/bitwarden/scripting.md)

---

### [`inject-env.sh`](inject-env.sh)

Inject secrets as environment variables into a child process **without writing them to disk or the shell history**. Supports both Vault and Bitwarden backends.

**Prerequisites:** `vault` CLI (for Vault backend) or `bw` CLI (for Bitwarden backend).

This is an executable script (not sourceable). Run it directly:

```bash
chmod +x snippets/inject-env.sh

# Run your app with Vault secrets in its environment
./snippets/inject-env.sh vault secret/myproject/prod -- ./my-app

# Run with Bitwarden secrets
./snippets/inject-env.sh bitwarden "prod-db" -- ./my-app

# Use an alias for convenience
alias inject-vault='./snippets/inject-env.sh vault'
inject-vault secret/myproject/prod -- node server.js
```

Secret names are uppercased automatically (e.g., `database_url` → `DATABASE_URL`).

> ⚠️ Secrets are scoped to the child process only. The parent shell environment is never modified.

**Related:** [env-files guide](../guides/general/env-files.md) · [Secret injector agent](../agents/secret-injector/)

---

### [`pre-commit-hook.sh`](pre-commit-hook.sh)

Git pre-commit hook that scans staged files for secrets before they are committed. Uses `gitleaks` if installed, with a regex fallback for common patterns (AWS keys, GitHub tokens, Stripe keys, JWT, private keys).

**Prerequisites:** `git`. Optionally `gitleaks` for more thorough scanning.

**Install as a hook:**

```bash
# Copy to your repo's hooks directory
cp snippets/pre-commit-hook.sh /path/to/your-repo/.git/hooks/pre-commit
chmod +x /path/to/your-repo/.git/hooks/pre-commit
```

**Install for all new repos (global):**

```bash
mkdir -p ~/.git-hooks
cp snippets/pre-commit-hook.sh ~/.git-hooks/pre-commit
chmod +x ~/.git-hooks/pre-commit
git config --global core.hooksPath ~/.git-hooks
```

**Run manually against staged files:**

```bash
bash snippets/pre-commit-hook.sh
```

**Related:** [Git security guide](../guides/general/git-security.md) · [Secret scanner agent](../agents/secret-scanner/)

---

### [`kubeconfig-merge.sh`](kubeconfig-merge.sh)

Safely merge kubeconfig files with conflict detection, backup creation, and dry-run support. Prevents accidentally overwriting contexts or mangling your `~/.kube/config`.

**Prerequisites:** `kubectl`.

**Key functions:**

| Function | Description |
|----------|-------------|
| `kubeconfig_merge` | Merge a kubeconfig file into `~/.kube/config` with backup |
| `kubeconfig_dry_run` | Preview what merging would produce without writing |
| `kubeconfig_validate` | Check a kubeconfig file is well-formed |
| `kubeconfig_list_contexts` | List all contexts across multiple kubeconfig files |

This is an executable script. Run it directly:

```bash
chmod +x snippets/kubeconfig-merge.sh

# Preview first (dry run)
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig --dry-run

# Merge (creates a timestamped backup automatically)
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig

# Merge into a specific output file
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig --output ~/.kube/config
```

**Related:** [Kubeconfig guide](../guides/kubernetes/kubeconfig.md)
