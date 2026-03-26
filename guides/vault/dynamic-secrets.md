# HashiCorp Vault — Dynamic Secrets

Dynamic secrets are generated on-demand by Vault and automatically expire. This eliminates long-lived credentials and reduces the blast radius of a compromise.

---

## Why Dynamic Secrets?

| Static credentials | Dynamic credentials |
|---|---|
| Long-lived, must be rotated manually | Auto-expire after a configurable TTL |
| Shared across services/users | Unique per request — full auditability |
| Revocation requires changing all users | Revoke a single lease instantly |

---

## Database Dynamic Credentials

Vault can generate short-lived database users. Supported: PostgreSQL, MySQL, MongoDB, MSSQL, Oracle, and more.

### Request credentials

```bash
vault read database/creds/<ROLE_NAME>
```

Example output:

```
Key                Value
---                -----
lease_id           database/creds/readonly/abc123
lease_duration     1h
lease_renewable    true
password           A1a-XkYzRt9mNqLp
username           v-token-readonly-xyz
```

Use these credentials immediately — they expire when the lease ends.

### Renew a lease

```bash
vault lease renew database/creds/readonly/<LEASE_ID>
```

### Revoke a lease immediately

```bash
vault lease revoke database/creds/readonly/<LEASE_ID>
```

### Use in a script

```bash
# Fetch creds, extract, use, then revoke
CREDS=$(vault read -format=json database/creds/<ROLE_NAME>)
LEASE_ID=$(echo "$CREDS" | jq -r '.lease_id')
DB_USER=$(echo "$CREDS" | jq -r '.data.username')
DB_PASS=$(echo "$CREDS" | jq -r '.data.password')

# Use the credentials...
psql "postgresql://${DB_USER}:${DB_PASS}@db.example.com:5432/mydb" -c "SELECT 1"

# Revoke when done
vault lease revoke "$LEASE_ID"
```

---

## AWS Dynamic Credentials

Vault can generate temporary AWS IAM users or assume roles.

### Request credentials

```bash
vault read aws/creds/<ROLE_NAME>
```

Example output:

```
Key                Value
---                -----
lease_id           aws/creds/s3-reader/xyz
lease_duration     768h
access_key         AKIAIOSFODNN7EXAMPLE
secret_key         wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
security_token     <token>
```

Use with the AWS CLI:

```bash
export AWS_ACCESS_KEY_ID="$access_key"
export AWS_SECRET_ACCESS_KEY="$secret_key"
export AWS_SESSION_TOKEN="$security_token"
aws s3 ls
```

---

## SSH Certificates (OTP / CA signing)

Vault can sign SSH certificates, granting time-limited access to target hosts.

### Request a signed certificate

```bash
vault write -field=signed_key ssh/sign/<ROLE_NAME> \
  public_key=@$HOME/.ssh/id_ed25519.pub > ~/.ssh/signed-cert.pub

ssh -i ~/.ssh/signed-cert.pub -i ~/.ssh/id_ed25519 user@host.example.com
```

---

## Lease Management

```bash
# List active leases
vault list sys/leases/lookup/database/creds/<ROLE_NAME>

# Revoke all leases for a role
vault lease revoke -prefix database/creds/<ROLE_NAME>

# Configure lease TTLs (admin operation)
vault write database/roles/<ROLE_NAME> default_ttl=1h max_ttl=24h
```

---

## Best Practices

- Request credentials as late as possible, revoke as early as possible
- Set `max_ttl` to the minimum necessary for your task
- Use `vault agent` or SDK lease renewal to keep long-running apps alive
- Log all lease requests — Vault audit logs provide a full audit trail
