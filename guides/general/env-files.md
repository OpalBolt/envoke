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

1. **Never commit `.env` to version control** — add `.env` to `.gitignore`
2. **Never send `.env` files over Slack, email, or chat**
3. **Always use `.env.example` or `.env.template`** — a version-controlled file with keys but no values
4. **Prefer runtime injection** over persistent `.env` files for production-adjacent environments

---

## .env.example Pattern

Check in a template with placeholder values:

```bash
# .env.example — commit this
DATABASE_URL=postgresql://user:password@localhost:5432/mydb
STRIPE_API_KEY=sk_test_<YOUR_KEY_HERE>
AWS_ACCESS_KEY_ID=<YOUR_AWS_KEY>
AWS_SECRET_ACCESS_KEY=<YOUR_AWS_SECRET>
```

Each developer creates their own `.env` locally from the template:

```bash
cp .env.example .env
# Then fill in real values from Vault or Bitwarden
```

---

## Loading .env Without Persisting

Use tools that load `.env` only for the duration of the command:

### direnv (recommended)

[direnv](https://direnv.net/) automatically loads `.envrc` when you enter a directory and unloads when you leave.

```bash
brew install direnv
# Add to shell: eval "$(direnv hook zsh)"

# .envrc
export DATABASE_URL=$(vault kv get -field=url secret/myproject/db)
export STRIPE_API_KEY=$(bw get password "stripe-api-key")
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

Instead of a `.env` file, inject at runtime from Vault:

```bash
# inject-env.sh — see snippets/
eval "$(vault kv get -format=json secret/myproject/dev | \
  jq -r '.data.data | to_entries[] | "export \(.key | ascii_upcase)=\(.value)"')"
```

Or use **Vault Agent** to automatically populate environment variables for a long-running process.

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
- [inject-env.sh snippet](../../snippets/inject-env.sh)
