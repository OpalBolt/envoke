# HashiCorp Vault — Reading and Writing Secrets

Vault's KV (key-value) secrets engine is the most common way to store and retrieve static secrets.

---

## Prerequisites

- Vault CLI installed and `VAULT_ADDR` set — see [setup.md](setup.md)
- Authenticated — see [authentication.md](authentication.md)
- A KV v2 mount (standard for most organisations — ask your Vault admin)

---

## KV v2 Basics

### Write a secret

```bash
vault kv put secret/myproject/db \
  username="app_user" \
  password="<YOUR_DB_PASSWORD>"
```

### Read a secret

```bash
vault kv get secret/myproject/db
```

Output:

```
====== Secret Path ======
secret/data/myproject/db

======= Metadata =======
Key              Value
---              -----
created_time     2024-01-15T10:00:00Z
version          1

====== Data ======
Key         Value
---         -----
password    <YOUR_DB_PASSWORD>
username    app_user
```

### Read a single field

```bash
vault kv get -field=password secret/myproject/db
```

Use in scripts (avoids printing to terminal):

```bash
DB_PASSWORD=$(vault kv get -field=password secret/myproject/db)
```

### List secrets at a path

```bash
vault kv list secret/myproject
```

### Update a secret (creates a new version)

```bash
vault kv patch secret/myproject/db password="<NEW_PASSWORD>"
```

### Delete a secret

```bash
# Soft delete (recoverable)
vault kv delete secret/myproject/db

# Permanently destroy all versions
vault kv destroy -versions=1,2 secret/myproject/db

# Remove all metadata and versions
vault kv metadata delete secret/myproject/db
```

---

## Working with JSON output

Useful for scripting and piping into tools like `jq`:

```bash
vault kv get -format=json secret/myproject/db | jq '.data.data'
```

Extract into environment variables without writing to disk:

```bash
eval "$(vault kv get -format=json secret/myproject/db | jq -r '.data.data | to_entries[] | "export \(.key | ascii_upcase)=\(.value)"')"
```

---

## KV v1 (legacy)

Some older Vault instances use KV v1. Commands differ slightly:

```bash
vault kv get kv/myproject/db       # v2 (standard)
vault read secret/myproject/db     # v1 (legacy)
vault write secret/myproject/db password="<PASSWORD>"  # v1 write
```

Ask your Vault admin which version your mount uses.

---

## Path Naming Conventions

Follow your organisation's agreed structure. A recommended pattern:

```
secret/<team>/<project>/<environment>/<secret-name>
```

Example:

```
secret/platform/payments-api/prod/stripe-api-key
secret/platform/payments-api/staging/stripe-api-key
```

---

## Access Policies

Your Vault token must have a policy granting access. If you receive `permission denied`:

1. Check the token policies: `vault token lookup`
2. Ask your Vault admin to grant access to the required path

---

## Examples

Full working Vault clients are available in the [examples](../../examples/) directory:

| Language | File |
|----------|------|
| Bash | [`examples/bash/vault-client.sh`](../../examples/bash/vault-client.sh) |
| Python | [`examples/python/vault_client.py`](../../examples/python/vault_client.py) |
| Go | [`examples/go/vault_client.go`](../../examples/go/vault_client.go) |
| TypeScript | [`examples/typescript/vault-client.ts`](../../examples/typescript/vault-client.ts) |

For shell-level secret injection (exporting Vault fields as env vars), see [`inject-env.sh`](../../snippets/inject-env.sh).

---

## Next Steps

- [Dynamic credentials](dynamic-secrets.md)
