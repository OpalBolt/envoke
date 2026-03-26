# Bitwarden Secrets Manager — SDK Guide

Bitwarden offers two separate products for credential storage. This guide covers
**Bitwarden Secrets Manager**, the product designed for application and CI/CD use.

---

## Bitwarden Password Manager vs Secrets Manager

| | Password Manager | Secrets Manager |
|---|---|---|
| **Purpose** | Human-managed credentials (logins, cards, notes) | Machine secrets (API keys, tokens, database credentials) |
| **Access method** | `bw` CLI, browser extension, mobile app | SDK / REST API via access tokens |
| **Credential type** | Master password + BW_SESSION | Service account access token |
| **Ideal for** | Developer personal vault, team password sharing | Application code, CI/CD pipelines, scripts |
| **SDKs available** | bw CLI only | Python, Go, TypeScript, Rust, C# |

> Use **Password Manager** + `bw` CLI for personal credentials and team vaults.
> Use **Secrets Manager** + SDK for any code that needs to retrieve secrets programmatically.

---

## Setup

### 1. Enable Secrets Manager

In the Bitwarden web app: **Organisation → Billing → Secrets Manager** (requires an Organisation plan).

### 2. Store secrets

**Organisation → Secrets Manager → Secrets → New Secret**

Each secret has:
- **Key** — human-readable name (e.g. `prod/database/password`)
- **Value** — the secret value
- **Note** — optional metadata
- **Project** — optional grouping

Note the secret's **UUID** for use in code.

### 3. Create a Service Account

**Organisation → Secrets Manager → Service Accounts → New**

A service account represents a machine identity (a specific app, CI job, or script).

1. Name it after the application: `my-app-prod`
2. Assign it read access to the projects/secrets it needs
3. Generate an **access token** — copy it now, it won't be shown again

### 4. Set environment variables

```bash
export BWS_ACCESS_TOKEN="0.your-access-token-here..."
export BWS_ORGANIZATION_ID="your-org-uuid-here"
```

For CI/CD: store these as encrypted CI secrets, not in `.env` files.

---

## SDK Quick Start

### Python

```bash
pip install bitwarden-sdk
```

```python
import os
from bitwarden_sdk import BitwardenClient, client_settings_from_dict

client = BitwardenClient(
    settings=client_settings_from_dict({
        "apiUrl": "https://api.bitwarden.com",
        "identityUrl": "https://identity.bitwarden.com",
        "userAgent": "my-app/1.0",
        "deviceType": "SDK",
    })
)

client.auth().login_access_token(
    os.environ["BWS_ACCESS_TOKEN"],
    os.environ.get("BWS_STATE_FILE", ""),  # optional state persistence
)

# Get a secret by UUID
secret = client.secrets().get("your-secret-uuid")
print(secret.data.value)

# List all secrets in the organisation (keys + IDs only, no values)
org_id = os.environ["BWS_ORGANIZATION_ID"]
secrets = client.secrets().list(org_id)
for s in secrets.data.data:
    print(f"{s.key}: {s.id}")
```

Full example: [`examples/python/bitwarden_client.py`](../../examples/python/bitwarden_client.py)

### Go

```bash
go get github.com/bitwarden/sdk-go
```

```go
import (
    "os"
    sdk "github.com/bitwarden/sdk-go"
)

apiURL := "https://api.bitwarden.com"
identityURL := "https://identity.bitwarden.com"

client, err := sdk.NewBitwardenClient(&apiURL, &identityURL)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

err = client.AccessTokenLogin(os.Getenv("BWS_ACCESS_TOKEN"), nil) // nil = no state file
if err != nil {
    log.Fatal(err)
}

// Get a secret by UUID
secret, err := client.Secrets().Get("your-secret-uuid")
if err != nil {
    log.Fatal(err)
}
fmt.Println(secret.Value)

// List secrets
secrets, err := client.Secrets().List(os.Getenv("BWS_ORGANIZATION_ID"))
if err != nil {
    log.Fatal(err)
}
for _, s := range secrets.Data {
    fmt.Printf("%s: %s\n", s.Key, s.ID)
}
```

Full example: [`examples/go/bitwarden_client.go`](../../examples/go/bitwarden_client.go)

### TypeScript / Node.js

```bash
npm install @bitwarden/sdk-napi
```

```typescript
import * as bitwarden from "@bitwarden/sdk-napi";

const client = new bitwarden.BitwardenClient({
    apiUrl: "https://api.bitwarden.com",
    identityUrl: "https://identity.bitwarden.com",
    userAgent: "my-app/1.0",
    deviceType: bitwarden.DeviceType.SDK,
});

await client.auth().loginAccessToken(
    process.env.BWS_ACCESS_TOKEN!,
    process.env.BWS_STATE_FILE ?? "",
);

// Get a secret by UUID
const secret = await client.secrets().get("your-secret-uuid");
console.log(secret.value);

// List secrets
const secrets = await client.secrets().list(process.env.BWS_ORGANIZATION_ID!);
for (const s of secrets.data) {
    console.log(`${s.key}: ${s.id}`);
}
```

Full example: [`examples/typescript/bitwarden-client.ts`](../../examples/typescript/bitwarden-client.ts)

---

## Getting a Secret by Key Name

The SDK retrieves secrets by UUID. To look up by human-readable key name, list
first and then fetch:

```python
def get_by_key(client, org_id, key):
    secrets = client.secrets().list(org_id)
    for s in secrets.data.data:
        if s.key == key:
            return client.secrets().get(s.id).data.value
    raise KeyError(f"Secret '{key}' not found")
```

**Tip**: Store secret UUIDs in your app config (not the values). This avoids
listing on every call and is more efficient.

---

## State File (optional)

The state file caches authentication state to disk, reducing round-trips on
repeated invocations:

```bash
export BWS_STATE_FILE="/run/secrets/bws-state"  # tmpfs recommended
```

**Do not use state files in Docker containers** unless the file is on a tmpfs
mount — the state file contains sensitive tokens.

---

## Self-Hosted Bitwarden

If using a self-hosted Bitwarden instance, override the API and identity URLs:

```python
client = BitwardenClient(
    settings=client_settings_from_dict({
        "apiUrl": "https://bitwarden.example.com/api",
        "identityUrl": "https://bitwarden.example.com/identity",
        ...
    })
)
```

```bash
export BITWARDEN_API_URL="https://bitwarden.example.com/api"
export BITWARDEN_IDENTITY_URL="https://bitwarden.example.com/identity"
```

---

## Security checklist

- [ ] Access tokens are stored as CI secrets or in Vault — never in `.env` files or source code
- [ ] Each application has its own service account with minimum required permissions
- [ ] Service accounts are scoped to specific projects, not the whole organisation
- [ ] State files (if used) are on tmpfs or deleted after use
- [ ] Access tokens are rotated when service accounts are decommissioned
- [ ] Secret UUIDs (not values) are what you commit to config files

---

## Related

- [Bitwarden Secrets Manager docs](https://bitwarden.com/help/secrets-manager-overview/)
- [SDK source — Python](https://github.com/bitwarden/sdk-python)
- [SDK source — Go](https://github.com/bitwarden/sdk-go)
- [Bitwarden setup guide](setup.md)
- [Bitwarden scripting guide (bw CLI)](scripting.md)
