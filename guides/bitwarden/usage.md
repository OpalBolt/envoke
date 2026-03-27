# Bitwarden CLI — Usage Guide

## Prerequisites

- Bitwarden CLI installed and logged in — see [setup.md](setup.md)
- `BW_SESSION` exported in your current shell

---

## Listing Items

```bash
# List all items
bw list items --session "$BW_SESSION"

# Search by name
bw list items --search "aws" --session "$BW_SESSION"

# Filter by collection
bw list items --collectionid "<COLLECTION_ID>" --session "$BW_SESSION"
```

Output is JSON. Use `jq` to extract fields:

```bash
bw list items --search "aws" --session "$BW_SESSION" | jq '.[0].login.password'
```

---

## Getting a Specific Item

### By name (search + select first match)

```bash
bw get item "aws-prod" --session "$BW_SESSION"
```

### By ID

```bash
bw get item "<ITEM_ID>" --session "$BW_SESSION"
```

### Extract just the password

```bash
bw get password "aws-prod" --session "$BW_SESSION"
```

### Extract a custom field

```bash
bw get item "stripe-api" --session "$BW_SESSION" | \
  jq -r '.fields[] | select(.name=="api_key") | .value'
```

### Extract a note

```bash
bw get notes "my-ssh-key" --session "$BW_SESSION"
```

---

## Creating Items

### Login item

```bash
bw get template item.login | jq '
  .name = "my-new-service" |
  .login.username = "admin" |
  .login.password = "<PASSWORD>"
' | bw encode | bw create item --session "$BW_SESSION"
```

### Secure note

```bash
bw get template item.securenote | jq '
  .name = "my-server-cert" |
  .notes = "<CERTIFICATE_CONTENTS>"
' | bw encode | bw create item --session "$BW_SESSION"
```

---

## Updating Items

```bash
# Get current item JSON, modify, and update
ITEM=$(bw get item "<ITEM_ID>" --session "$BW_SESSION")
echo "$ITEM" | jq '.login.password = "<NEW_PASSWORD>"' | \
  bw encode | bw edit item "<ITEM_ID>" --session "$BW_SESSION"
```

---

## Organisations and Collections

```bash
# List organisations
bw list organizations --session "$BW_SESSION"

# List collections
bw list collections --session "$BW_SESSION"

# List items in a specific collection
bw list items --collectionid "<COLLECTION_ID>" --session "$BW_SESSION"
```

---

## Syncing

Always sync before reading critical secrets to ensure you have the latest version:

```bash
bw sync --session "$BW_SESSION"
```

---

## Exporting (for backup — handle with care)

```bash
# Encrypted export (safest)
bw export --format encrypted_json --session "$BW_SESSION"

# JSON export — unencrypted, treat like a plaintext file
bw export --format json --session "$BW_SESSION"
```

> ⚠️ An unencrypted JSON export contains **all your plaintext secrets**. Store it encrypted (e.g., `gpg --symmetric export.json`) and delete the plaintext immediately.

---

## Snippet and SDK

For shell scripts, use the `bw` CLI directly or [`resolve-env-refs.sh`](../../snippets/resolve-env-refs.sh) to resolve `bw://` references in `.env` files. See [snippets/README.md](../../snippets/README.md) for usage.

For **application code** (Python, Go, TypeScript), use the **Bitwarden Secrets Manager SDK** instead of the CLI. The SDK uses machine access tokens, not BW_SESSION, and is designed for programmatic use:

→ [Bitwarden Secrets Manager SDK guide](secrets-manager-sdk.md)

---

## Next Steps

- [Using Bitwarden in scripts](scripting.md)
- [Bitwarden Secrets Manager SDK](secrets-manager-sdk.md)
