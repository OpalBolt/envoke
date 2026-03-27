# HashiCorp Vault — Authentication Methods

Vault supports multiple auth methods. This guide covers the three most common for consultants working locally.

---

## 1. Token Auth (simplest)

Best for: quick access, CI/CD pipelines, service accounts.

```bash
vault login <YOUR_VAULT_TOKEN>
```

Or set the token directly:

```bash
export VAULT_TOKEN="hvs.XXXXXXXXXXXXXXXX"
```

Check current token info:

```bash
vault token lookup
```

> ⚠️ Tokens are stored in `~/.vault-token`. This file is readable by any process running as your user. Prefer OIDC for human login.

---

## 2. OIDC / SSO (recommended for humans)

Best for: interactive login via your organisation's identity provider (Okta, Azure AD, Google Workspace).

```bash
vault login -method=oidc
```

This opens a browser window for SSO login. On success, Vault writes a short-lived token to `~/.vault-token`.

Specify a role if required:

```bash
vault login -method=oidc role="developer"
```

List available roles:

```bash
vault list auth/oidc/role
```

---

## 3. AppRole (for automated processes / scripts)

Best for: scripts, agents, CI runners — anything that cannot do interactive browser login.

### Obtain credentials from your Vault admin

```
Role ID:   <ROLE_ID>
Secret ID: <SECRET_ID>   # treat this like a password — store in Bitwarden
```

### Login

```bash
vault write auth/approle/login \
  role_id="<ROLE_ID>" \
  secret_id="<SECRET_ID>"
```

This returns a `client_token`. Export it:

```bash
export VAULT_TOKEN="$(vault write -field=token auth/approle/login \
  role_id="<ROLE_ID>" \
  secret_id="<SECRET_ID>")"
```

### Secret ID best practices

- Secret IDs are single-use or have a short TTL — regenerate as needed
- Never hardcode Secret IDs in scripts; retrieve them from Bitwarden at runtime
- Use response-wrapping for secure delivery: `vault write -wrap-ttl=30s -f auth/approle/role/<ROLE>/secret-id`

---

## Token Lifecycle

| Action | Command |
|--------|---------|
| View token details | `vault token lookup` |
| Renew token TTL | `vault token renew` |
| Revoke token | `vault token revoke` |
| Revoke all tokens for a role | `vault token revoke -mode=path auth/approle/login` |

---

## Snippet

[`resolve-env-refs.sh`](../../snippets/resolve-env-refs.sh) handles vault:// references directly in `.env` files and supports all three auth methods above as prerequisites. Set `VAULT_TOKEN` before using vault:// references. See [snippets/README.md](../../snippets/README.md) for the full pattern.

---

## Next Steps

- [Reading and writing secrets](read-write-secrets.md)
- [Dynamic credentials](dynamic-secrets.md)
