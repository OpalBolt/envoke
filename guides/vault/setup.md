# HashiCorp Vault — Setup Guide

## Prerequisites

- Linux/macOS workstation
- `curl` or a package manager (Homebrew, apt, yum)

---

## 1. Install the Vault CLI

### macOS (Homebrew)

```bash
brew tap hashicorp/tap
brew install hashicorp/tap/vault
```

### Linux (apt)

```bash
wget -O - https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install vault
```

### Linux (binary)

```bash
curl -fsSL https://releases.hashicorp.com/vault/1.16.0/vault_1.16.0_linux_amd64.zip -o vault.zip
unzip vault.zip
sudo mv vault /usr/local/bin/
rm vault.zip
```

Verify installation:

```bash
vault version
```

---

## 2. Configure Environment Variables

Add to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.):

```bash
# Vault server address — change to your organisation's Vault endpoint
export VAULT_ADDR="https://vault.example.com:8200"

# (Optional) Namespace for Vault Enterprise
export VAULT_NAMESPACE="admin"

# (Optional) Path to CA cert if using a self-signed TLS certificate
export VAULT_CACERT="/path/to/ca.crt"
```

Reload your shell:

```bash
source ~/.zshrc  # or source ~/.bashrc
```

---

## 3. Verify Connectivity

```bash
vault status
```

Expected output (when unsealed):

```
Key             Value
---             -----
Seal Type       shamir
Initialized     true
Sealed          false
...
```

---

## 4. (Optional) Local Dev Vault

For local development and testing only — **never for production secrets**:

```bash
vault server -dev -dev-root-token-id="root"
```

In a separate terminal:

```bash
export VAULT_ADDR="http://127.0.0.1:8200"
export VAULT_TOKEN="root"
vault status
```

> ⚠️ Dev mode stores everything in memory. All data is lost on restart.

---

## Next Steps

- [Authentication methods](authentication.md)
- [Reading and writing secrets](read-write-secrets.md)
