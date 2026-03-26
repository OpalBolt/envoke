# Secret Rotation — Practices and Reminders

Regular rotation limits the window of exposure if a secret is compromised. This guide covers when and how to rotate secrets stored in Vault and Bitwarden.

---

## Why Rotate?

- Limits blast radius if a secret is silently leaked
- Compliance requirements (SOC 2, ISO 27001, PCI-DSS mandate periodic rotation)
- Offboarding — rotate any secret a departing team member had access to
- After a suspected or confirmed breach

---

## Rotation Triggers

| Event | Action |
|---|---|
| Team member offboarding | Rotate all secrets they had access to |
| Suspected compromise | Rotate immediately, audit logs, notify affected parties |
| Repository exposure | Rotate all secrets that were in the repo |
| Periodic schedule | Rotate per policy (typically 90 days for API keys) |
| Contractor engagement ends | Rotate all project secrets |

---

## HashiCorp Vault

### Prefer dynamic secrets over manual rotation

Dynamic credentials (database, AWS, SSH) expire automatically — no manual rotation needed. See [dynamic-secrets.md](../vault/dynamic-secrets.md).

### Rotate static KV secrets

```bash
# Update the secret — Vault v2 creates a new version
vault kv patch secret/myproject/api api_key="<NEW_KEY>"

# Or replace entirely
vault kv put secret/myproject/api api_key="<NEW_KEY>"

# Verify new version
vault kv metadata get secret/myproject/api
```

### Rotate encryption keys (admin operation)

```bash
# Rotate a transit engine key
vault write -f transit/keys/<KEY_NAME>/rotate

# Set minimum version for decryption
vault write transit/keys/<KEY_NAME>/config min_decryption_version=2
```

### Audit who had access before rotation

```bash
# View audit log (requires audit backend enabled)
vault audit list
cat /var/log/vault/audit.log | jq 'select(.request.path | contains("myproject/api"))'
```

---

## Bitwarden

### Update an item

```bash
# Get current item
ITEM=$(bw get item "my-api-key" --session "$BW_SESSION")
ITEM_ID=$(echo "$ITEM" | jq -r '.id')

# Update password
echo "$ITEM" | jq '.login.password = "<NEW_PASSWORD>"' | \
  bw encode | bw edit item "$ITEM_ID" --session "$BW_SESSION"

# Sync to confirm
bw sync --session "$BW_SESSION"
```

### Track rotation history

Use a custom field to record the last rotation date:

```bash
echo "$ITEM" | jq '
  .login.password = "<NEW_PASSWORD>" |
  .fields += [{"name": "last_rotated", "value": "2024-01-15", "type": 0}]
' | bw encode | bw edit item "$ITEM_ID" --session "$BW_SESSION"
```

---

## Rotation Checklist

For each secret being rotated:

- [ ] Generate new credential in the target system (AWS IAM, database, API portal, etc.)
- [ ] Update in Vault (`vault kv put`) or Bitwarden (`bw edit`)
- [ ] Update all services/apps that use the secret (rolling deploy if needed)
- [ ] Verify the new secret works in all affected services
- [ ] Revoke / delete the old credential in the target system
- [ ] Update rotation log / document the rotation date
- [ ] Notify relevant team members that rotation occurred

---

## Automated Rotation Reminders

### Shell alias to check secret age (Vault)

```bash
vault_check_rotation() {
  local path="$1"
  local max_days="${2:-90}"
  
  local created
  created=$(vault kv metadata get -format=json "$path" | \
    jq -r '.data.versions | to_entries | sort_by(.key | tonumber) | last | .value.created_time')
  
  local age_days=$(( ($(date +%s) - $(date -d "$created" +%s)) / 86400 ))
  
  if [[ $age_days -gt $max_days ]]; then
    echo "⚠️  $path is $age_days days old (max $max_days days) — rotation recommended"
  else
    echo "✅  $path is $age_days days old — OK"
  fi
}
```

### Calendar reminders

Add rotation reminders to your calendar for secrets that require manual rotation:

- API keys: every 90 days
- Database passwords: every 90 days
- SSH keys: every 365 days
- TLS certificates: before expiry (automate with cert-manager or Let's Encrypt)

---

## Related

- [Dynamic secrets (auto-expiry)](../vault/dynamic-secrets.md)
- [Git security (post-commit cleanup)](git-security.md)
