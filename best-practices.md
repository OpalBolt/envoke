# Best Practices — Secure Handling of Secrets

A quick reference for consultants working with sensitive credentials.

---

## ✅ Do

### Storage
- Store secrets in **Vault** or **Bitwarden** — never in plaintext files
- Use **short-lived / dynamic credentials** where possible (Vault dynamic secrets)
- Use **`.env.example`** (with placeholders) instead of committing real `.env` files
- Encrypt files before sharing if you must transfer credentials (GPG, age)

### Runtime
- Inject secrets as **environment variables at runtime**, not baked into config files
- Use `eval "$(vault kv get ...)"` or `inject-env.sh` to scope secrets to a single process
- **Unset** sensitive environment variables after use: `unset API_KEY`
- Use a `trap ... EXIT` to clean up secrets in scripts

### Git
- Add `.env`, `*.pem`, `*.key`, `kubeconfig` to **`.gitignore`** (and global gitignore)
- Install **pre-commit hooks** with `gitleaks` or `detect-secrets` on every project
- **Rotate immediately** if a secret is accidentally committed — don't just remove it from history

### Operations
- **Lock your Bitwarden vault** when stepping away: `bw lock`
- **Rotate secrets** on a schedule (API keys: 90 days; DB passwords: 90 days)
- **Rotate all secrets** when a team member is offboarded
- Keep **`HISTCONTROL=ignorespace`** and prefix sensitive commands with a space

---

## ❌ Don't

### Version Control
- ~~Commit secrets to version control~~ — even private repos, even "just temporarily"
- ~~Use the same secret across multiple projects or clients~~
- ~~Include secrets in `Dockerfile`, CI config, or build logs~~

### Communication
- ~~Share secrets over Slack, email, Teams, or Jira~~ — use Vault/Bitwarden sharing
- ~~Send `.env` files as attachments~~

### Shell
- ~~Type secrets directly at the shell prompt~~ — they end up in `~/.bash_history`
- ~~Pass secrets as CLI arguments~~ — they appear in `ps aux` output
- ~~`echo` or `print` a secret value in logs~~

### Code
- ~~Hardcode credentials in source code~~
- ~~Store secrets in comments~~
- ~~Log secret values, even at debug level~~

---

## Decision Tree

```
Where do I store this secret?
├─ It's shared across a team / project → HashiCorp Vault
│   ├─ It's a database credential → Use dynamic secrets (auto-expire)
│   ├─ It's a cloud provider key → Use dynamic secrets (AWS, GCP)
│   └─ It's a static API key → Vault KV v2, rotate every 90 days
│
└─ It's personal / my own account → Bitwarden
    ├─ It's a login password → Bitwarden login item
    ├─ It's an SSH key → Bitwarden secure note
    └─ It's an API key for a personal account → Bitwarden login item (custom field)
```

---

## Quick Reference

| Task | Command |
|---|---|
| Write to Vault | `vault kv put secret/path key=value` |
| Read from Vault | `vault kv get -field=key secret/path` |
| Unlock Bitwarden | `export BW_SESSION=$(bw unlock --raw)` |
| Get from Bitwarden | `bw get password "item-name"` |
| Inject into process | `./snippets/inject-env.sh vault secret/path -- command` |
| Scan for leaked secrets | `gitleaks protect --staged` |
| Check shell history for secrets | `history \| grep -iE "key\|secret\|password\|token"` |
| Rotate a Vault secret | `vault kv patch secret/path key=<new-value>` |

---

## Guides

| Topic | Guide |
|---|---|
| Vault setup | [guides/vault/setup.md](guides/vault/setup.md) |
| Vault authentication | [guides/vault/authentication.md](guides/vault/authentication.md) |
| Vault dynamic secrets | [guides/vault/dynamic-secrets.md](guides/vault/dynamic-secrets.md) |
| Bitwarden setup | [guides/bitwarden/setup.md](guides/bitwarden/setup.md) |
| Bitwarden scripting | [guides/bitwarden/scripting.md](guides/bitwarden/scripting.md) |
| Git security | [guides/general/git-security.md](guides/general/git-security.md) |
| .env file security | [guides/general/env-files.md](guides/general/env-files.md) |
| Shell security | [guides/general/shell-security.md](guides/general/shell-security.md) |
| Secret rotation | [guides/general/secret-rotation.md](guides/general/secret-rotation.md) |
| Kubeconfig management | [guides/kubernetes/kubeconfig.md](guides/kubernetes/kubeconfig.md) |
| Kubernetes secrets | [guides/kubernetes/k8s-secrets.md](guides/kubernetes/k8s-secrets.md) |
