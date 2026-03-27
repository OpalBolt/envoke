# Secure Handling of .env Files

`.env` files are convenient for local development but are a common source of accidental secret exposure.

---

## The Problem with .env Files

- Developers often copy `.env` files from colleagues over Slack/email (unencrypted channels)
- `.env` files get committed to git accidentally
- They persist on disk in plaintext indefinitely
- They are shared across projects, meaning one leaked file exposes multiple services

---

## Rules

1. **Never put secret values directly in `.env` files** — use references to Vault or Bitwarden instead
2. **Never commit `.env` to version control** — add `.env` to `.gitignore`
3. **Never send `.env` files over Slack, email, or chat**
4. **Always use `.env.example`** — a version-controlled file with reference syntax, not real values
5. **Prefer runtime injection** over persistent `.env` files for production-adjacent environments

---

## .env Reference Pattern (Recommended)

Instead of storing secrets in `.env` files, store **references** that are resolved at runtime by your secret manager. This file is safe to commit because it contains no real values.

```bash
# .env.example — safe to commit (references, not secrets)
DATABASE_URL=bw://prod-db/password
DATABASE_USER=bw://prod-db/username
STRIPE_SECRET_KEY=bw://stripe-api/field:secret_key
INTERNAL_API_TOKEN=vault://secret/myproject/app#api_token
```

Resolve references at runtime using [`snippets/resolve-env-refs.sh`](../../snippets/resolve-env-refs.sh):

```bash
# Mode 1 — exec (recommended): inject directly, no shell exposure
./snippets/resolve-env-refs.sh .env.example -- node server.js
./snippets/resolve-env-refs.sh .env.example -- python app.py

# Mode 2 — source into current shell (safe alternative to eval)
source <(./snippets/resolve-env-refs.sh .env.example)
```

> ⚠️ **Never use `eval "$(./resolve-env-refs.sh ...)"`** — `eval` re-interprets secret values
> as shell code, enabling injection attacks if a secret contains `$(...)` or backticks.
> Use `source <(...)` instead.

### Reference syntax

**Bitwarden Password Manager** (resolved via `bw` CLI):

| Reference | Retrieves |
|-----------|-----------|
| `bw://item-name` | Password field (default) |
| `bw://item-name/password` | Password field |
| `bw://item-name/username` | Username field |
| `bw://item-name/note` | Notes field |
| `bw://item-name/field:fname` | Custom field named `fname` |

**HashiCorp Vault** (resolved via `vault` CLI):

| Reference | Retrieves |
|-----------|-----------|
| `vault://secret/path#field` | Single field from KV path |
| `vault://secret/path` | All fields from KV path as `KEY=value` |

---

## .env.example Pattern (legacy — without references)

If your team is not yet using the reference pattern, the minimum is to check in placeholder values with no real secrets:

```bash
# .env.example — commit this (old pattern — no references, no real values)
DATABASE_URL=postgresql://user:password@localhost:5432/mydb
STRIPE_API_KEY=sk_test_<YOUR_KEY_HERE>
AWS_ACCESS_KEY_ID=<YOUR_AWS_KEY>
AWS_SECRET_ACCESS_KEY=<YOUR_AWS_SECRET>
```

Each developer then creates their own `.env` locally from the template, filling in values retrieved from Vault or Bitwarden. **Migrate to the reference pattern above** to eliminate this manual step.

---

## Loading .env Without Persisting

Use tools that load `.env` only for the duration of the command:

### direnv (recommended)

[direnv](https://direnv.net/) automatically loads `.envrc` when you enter a directory and unloads when you leave.

```bash
brew install direnv
# Add to shell: eval "$(direnv hook zsh)"
```

Pin the resolver to a specific commit SHA (immutable) and validate with an SRI hash.
Generate the hash: `shasum -a 256 snippets/resolve-env-refs.sh | awk '{print $1}' | xxd -r -p | base64`

```bash
# .envrc — resolves bw:// and vault:// references at directory entry
source_url "https://raw.githubusercontent.com/eficode/secure-handling-of-secrets/<SHA>/snippets/resolve-env-refs.sh" \
  "sha256-<HASH>"
source <(resolve_env_file .env)
```

```bash
direnv allow .   # grant permission once per project
```

### dotenv-cli

```bash
npm install -g dotenv-cli
dotenv -e .env -- node server.js
```

### env command

```bash
# Inline, no file
env DATABASE_URL="$(vault kv get -field=url secret/myproject/db)" node server.js
```

---

## Injecting from Vault at Runtime

Instead of a `.env` file, inject directly from Vault into a child process:

```bash
# Exec mode — secrets are passed directly to the process, never touch your shell
./snippets/resolve-env-refs.sh .env -- node server.js

# Or use Vault Agent to automatically populate environment variables for a long-running process
```

---

## Encrypting .env Files (for sharing or backup)

If you must share or store a `.env` file, encrypt it:

### GPG symmetric encryption

```bash
# Encrypt
gpg --symmetric --cipher-algo AES256 .env
# Produces .env.gpg — share/store this

# Decrypt
gpg --decrypt .env.gpg > .env
```

### age

```bash
brew install age

# Encrypt with a recipient's public key
age -r <RECIPIENT_PUBLIC_KEY> -o .env.age .env

# Decrypt
age --decrypt -i ~/.age/key.txt .env.age > .env
```

---

## Secrets in Docker

Never bake secrets into Docker images:

```dockerfile
# ❌ Never do this
ENV AWS_SECRET_ACCESS_KEY=secretvalue
RUN echo "password=secret" >> config.ini
```

Instead:

```bash
# Pass at runtime only
docker run -e AWS_ACCESS_KEY_ID="$(vault kv get -field=key secret/aws)" myapp

# Or use Docker secrets (Swarm/Compose)
docker secret create db_password - < <(vault kv get -field=password secret/db)
```

---

## Related

- [Shell security](shell-security.md)
- [resolve-env-refs.sh snippet](../../snippets/resolve-env-refs.sh)
