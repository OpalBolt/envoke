# Secure Handling of Secrets

A practical reference for consultants who need to store, retrieve, and inject secrets safely — covering API keys, tokens, passwords, kubeconfig files, certificates, and other sensitive credentials.

> **Approved backends:** [HashiCorp Vault](https://www.vaultproject.io/) for team/project secrets · [Bitwarden](https://bitwarden.com/) for personal credentials

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [When to Use a Snippet vs an Example](#when-to-use-a-snippet-vs-an-example)
- [Snippets](#snippets)
- [Examples](#examples)
- [Guides](#guides)
- [Best Practices](#best-practices)
- [Getting Started](#getting-started)
- [Contributing](#contributing)

---

## Problem Statement

Consultants handle credentials across many projects and clients. Without clear tooling and guidance, secrets end up in plaintext files, shell histories, dotfiles, or accidentally committed to version control — creating real security and compliance risk.

This repository standardises how to store, retrieve, and inject secrets using Vault and Bitwarden, and provides ready-to-use code for the most common scenarios.

---

## When to Use a Snippet vs an Example

| | [Snippets](#snippets) | [Examples](#examples) |
|---|---|---|
| **What it is** | A small, focused shell function or script you `source` or copy-paste into your own work | A complete, runnable program in a specific language |
| **When to use** | You need a single operation in a shell script or terminal session | You're integrating secret retrieval into application code |
| **Scope** | One task (e.g. unlock vault, get a password, inject env) | Full client pattern including error handling, session management, and typed interfaces |
| **How to use** | `source snippets/bw-get-secret.sh` then call the function | Copy the file into your project, install dependencies, adapt as needed |
| **Languages** | Bash only | Bash · Python · Go · TypeScript |

**Rule of thumb:** reach for a snippet when you're automating something at the shell level. Use an example when you need secret retrieval inside application code.

---

## Snippets

Snippets live in [`snippets/`](snippets/). Each file is self-contained and designed to be `source`d in shell scripts or used as-is.

### `vault-login.sh` — Vault authentication helpers

```bash
source snippets/vault-login.sh

vault_login_oidc              # browser-based OIDC login
vault_login_approle "$ROLE_ID" "$SECRET_ID"   # non-interactive AppRole (CI/CD)
vault_login_token  "$CI_VAULT_TOKEN"          # use a known token
vault_token_renew                              # extend TTL before it expires
vault_logout                                   # revoke token, clear env
```

Use this when: you need to authenticate to Vault before running a script and want consistent error handling without reimplementing auth logic every time.

---

### `bw-get-secret.sh` — Bitwarden secret retrieval helpers

```bash
source snippets/bw-get-secret.sh

bw_ensure_unlocked                        # prompts for unlock if BW_SESSION is missing/expired
bw_get_password  "github-token"           # retrieve a login password by item name
bw_get_username  "github-token"           # retrieve the username
bw_get_field     "aws-prod" "access_key"  # retrieve a named custom field
bw_get_note      "prod-tls-cert"          # retrieve the notes field (for certs / keys)
bw_get_totp      "my-2fa-login"           # get the current TOTP code
bw_lock                                   # lock vault and unset BW_SESSION
```

Uses the [Bitwarden CLI (`bw`)](https://github.com/bitwarden/clients) under the hood.
Install: `brew install bitwarden-cli` or see [guides/bitwarden/setup.md](guides/bitwarden/setup.md).

Use this when: you need a single secret value in a shell script without pulling in a full example.

---

### `inject-env.sh` — Inject secrets as environment variables into a process

```bash
# Inject all fields from a Vault path as env vars into a command
./snippets/inject-env.sh vault secret/myproject/dev -- node server.js

# Inject from a Bitwarden item
./snippets/inject-env.sh bitwarden "prod-api-service" -- python app.py
```

The secrets are **never written to disk or shell history** — they are passed directly to the child process via `exec env`. When the process exits, the secrets are gone.

Use this when: you want to run a local service with real credentials without exporting them to your shell.

---

### `pre-commit-hook.sh` — Git pre-commit secret scanner

```bash
# Install into your repo
cp snippets/pre-commit-hook.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

Blocks commits that contain patterns matching API keys, tokens, passwords, and private keys. Wraps `gitleaks` if available, falls back to regex patterns.

Use this when: setting up a new project and you want a safety net before `git push`.

---

### `kubeconfig-merge.sh` — Safe kubeconfig merge

```bash
./snippets/kubeconfig-merge.sh /path/to/new-cluster.yaml
```

Merges a new kubeconfig into `~/.kube/config` with a backup, deduplication, and conflict detection.

Use this when: onboarding to a new cluster and you don't want to clobber your existing config.

---

## Examples

Examples live in [`examples/`](examples/). Each is a complete, runnable client you can copy into your project. They handle session management, error handling, and typed interfaces — things snippets don't cover.

### When to use an example over a snippet

- You're writing application code (not a shell script)
- You need typed access to secrets with proper error handling
- You want a pattern you can extend (e.g. caching, retries, multiple backends)

---

### Bash — [`examples/bash/`](examples/bash/)

| File | Description |
|---|---|
| [`vault-client.sh`](examples/bash/vault-client.sh) | Read/write/delete KV secrets via Vault CLI |
| [`bitwarden-client.sh`](examples/bash/bitwarden-client.sh) | Full Bitwarden CLI client — get password, field, note, TOTP, export as env |

**Bitwarden dependency:** [Bitwarden CLI (`bw`)](https://github.com/bitwarden/clients)
```bash
brew install bitwarden-cli    # macOS
npm install -g @bitwarden/cli # cross-platform
```

---

### Python — [`examples/python/`](examples/python/)

| File | Description |
|---|---|
| [`vault_client.py`](examples/python/vault_client.py) | Read/write/delete KV secrets using [`hvac`](https://github.com/hvac/hvac) |
| [`bitwarden_client.py`](examples/python/bitwarden_client.py) | Bitwarden secrets via the `bw` CLI subprocess |
| [`requirements.txt`](examples/python/requirements.txt) | `hvac` and dependencies |

```bash
pip install -r examples/python/requirements.txt

# Vault
export VAULT_ADDR=https://vault.example.com:8200
export VAULT_TOKEN=$(vault print token)
python examples/python/vault_client.py

# Bitwarden — uses the official CLI under the hood
# See: https://github.com/bitwarden/clients
export BW_SESSION=$(bw unlock --raw)
python examples/python/bitwarden_client.py
```

---

### Go — [`examples/go/`](examples/go/)

| File | Description |
|---|---|
| [`vault_client.go`](examples/go/vault_client.go) | Read/write/delete KV secrets using the [Vault Go SDK](https://github.com/hashicorp/vault-client-go) |
| [`bitwarden_client.go`](examples/go/bitwarden_client.go) | Bitwarden secrets via the `bw` CLI subprocess |
| [`go.mod`](examples/go/go.mod) | Module definition |

```bash
cd examples/go && go mod tidy

# Vault
VAULT_ADDR=https://vault.example.com:8200 VAULT_TOKEN=... go run vault_client.go

# Bitwarden — wraps the official CLI
# See: https://github.com/bitwarden/clients
BW_SESSION=$(bw unlock --raw) go run bitwarden_client.go
```

---

### TypeScript — [`examples/typescript/`](examples/typescript/)

| File | Description |
|---|---|
| [`vault-client.ts`](examples/typescript/vault-client.ts) | Read/write/delete KV secrets using [`node-vault`](https://github.com/kr1sp1n/node-vault) |
| [`bitwarden-client.ts`](examples/typescript/bitwarden-client.ts) | Bitwarden secrets via the `bw` CLI subprocess — typed interfaces |
| [`package.json`](examples/typescript/package.json) | `node-vault`, TypeScript, ts-node |

```bash
cd examples/typescript && npm install

# Vault
VAULT_ADDR=https://vault.example.com:8200 VAULT_TOKEN=... npx ts-node vault-client.ts

# Bitwarden — uses the official CLI under the hood
# Install CLI: https://github.com/bitwarden/clients
export BW_SESSION=$(bw unlock --raw)
npx ts-node bitwarden-client.ts
```

---

## Guides

Detailed step-by-step guides in [`guides/`](guides/).

### HashiCorp Vault

| Guide | Description |
|---|---|
| [setup.md](guides/vault/setup.md) | Install and configure the Vault CLI |
| [authentication.md](guides/vault/authentication.md) | Token, OIDC, and AppRole auth methods |
| [read-write-secrets.md](guides/vault/read-write-secrets.md) | Store and retrieve secrets with KV v2 |
| [dynamic-secrets.md](guides/vault/dynamic-secrets.md) | Short-lived database and cloud credentials |

### Bitwarden

| Guide | Description |
|---|---|
| [setup.md](guides/bitwarden/setup.md) | Install the Bitwarden CLI and authenticate |
| [usage.md](guides/bitwarden/usage.md) | Store and retrieve secrets with `bw` |
| [scripting.md](guides/bitwarden/scripting.md) | Using Bitwarden in shell scripts and CI |

### General

| Guide | Description |
|---|---|
| [git-security.md](guides/general/git-security.md) | Pre-commit hooks, `.gitignore`, `gitleaks` |
| [env-files.md](guides/general/env-files.md) | Secure handling of `.env` files |
| [shell-security.md](guides/general/shell-security.md) | Avoid secrets in history and `ps` output |
| [secret-rotation.md](guides/general/secret-rotation.md) | Rotation practices and schedules |

### Kubernetes

| Guide | Description |
|---|---|
| [kubeconfig.md](guides/kubernetes/kubeconfig.md) | Secure kubeconfig management |
| [k8s-secrets.md](guides/kubernetes/k8s-secrets.md) | Working with Kubernetes secrets locally |

---

## Best Practices

See [`best-practices.md`](best-practices.md) for the full reference including a decision tree for choosing where to store a secret, a do/don't checklist, and a quick-reference command table.

### Quick decision tree

```
Where do I store this secret?
├─ Shared across a team or project → HashiCorp Vault
│   ├─ Database credential          → Dynamic secrets (auto-expire)
│   ├─ Cloud provider key           → Dynamic secrets (AWS/GCP)
│   └─ Static API key               → Vault KV v2, rotate every 90 days
│
└─ Personal / my own account        → Bitwarden
    ├─ Login password               → Bitwarden login item
    ├─ SSH or TLS key               → Bitwarden secure note
    └─ Personal API key             → Bitwarden login item (custom field)
```

### Never do this

- Commit secrets to version control — even in private repos, even "just temporarily"
- Pass secrets as CLI arguments — they appear in `ps aux`
- Store secrets in shell history — use `HISTCONTROL=ignorespace` and prefix with a space
- Hardcode credentials in source code, Dockerfiles, or CI config

---

## Getting Started

1. **Choose your backend** — team secret? Use Vault. Personal credential? Use Bitwarden.
2. **Install the CLI** — [guides/vault/setup.md](guides/vault/setup.md) or [guides/bitwarden/setup.md](guides/bitwarden/setup.md)
3. **Block accidental commits** — install the pre-commit hook: `cp snippets/pre-commit-hook.sh .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit`
4. **Grab a snippet or example** — shell script work? Start in `snippets/`. Application code? Start in `examples/<your-language>/`.

---

## AI Agents

Automated helpers in [`agents/`](agents/):

| Agent | Description |
|---|---|
| [secret-scanner](agents/secret-scanner/README.md) | Scan a local repo for leaked secrets |
| [secret-injector](agents/secret-injector/README.md) | Inject secrets from Vault or Bitwarden into an environment |

See [agents/README.md](agents/README.md) for setup and usage.

---

## Contributing

1. Create a branch from `main`
2. Add or update guides/snippets/examples following the existing structure
3. Ensure no real secrets appear anywhere — use `<YOUR_TOKEN>`, `<VAULT_ADDR>`, etc. as placeholders
4. Open a pull request for review
