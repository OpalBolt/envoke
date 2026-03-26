# Bitwarden CLI — Setup Guide

## Prerequisites

- Node.js ≥ 18 (for `npm` install) OR download the standalone binary
- A Bitwarden account (self-hosted or bitwarden.com)

---

## 1. Install the Bitwarden CLI

### npm (all platforms)

```bash
npm install -g @bitwarden/cli
```

### Homebrew (macOS)

```bash
brew install bitwarden-cli
```

### Standalone binary

Download from [https://bitwarden.com/help/cli/#download-and-install](https://bitwarden.com/help/cli/#download-and-install), make it executable:

```bash
chmod +x bw
sudo mv bw /usr/local/bin/
```

Verify:

```bash
bw --version
```

---

## 2. Configure the Server (if self-hosted)

```bash
bw config server https://vault.example.com
```

For bitwarden.com (default), no configuration needed.

---

## 3. Log In

```bash
bw login
```

You will be prompted for your email and master password. On success, you receive a **session key** — keep this safe, it grants full vault access for the session.

### Login with API key (for automation)

1. In the Bitwarden web app: **Account Settings → Security → API Key**
2. Note your `client_id` and `client_secret`

```bash
export BW_CLIENTID="<YOUR_CLIENT_ID>"
export BW_CLIENTSECRET="<YOUR_CLIENT_SECRET>"
bw login --apikey
```

---

## 4. Unlock the Vault

After login, unlock to get a session key for subsequent commands:

```bash
export BW_SESSION=$(bw unlock --passwordenv BW_PASSWORD --raw)
```

Or interactively:

```bash
export BW_SESSION=$(bw unlock --raw)
```

> ⚠️ The session key grants full vault access. Do not log it, commit it, or pass it as a CLI argument visible in `ps` output. Export it as an environment variable only.

---

## 5. Verify

```bash
bw status
bw sync          # sync latest from server
bw list items --search "github" --session "$BW_SESSION"
```

---

## 6. Lock and Logout

```bash
bw lock          # lock vault (keeps login state)
bw logout        # full logout
```

Lock your vault whenever you step away from your workstation. Consider adding `bw lock` to your screen-lock trigger.

---

## Shell Profile Integration

Add to `~/.zshrc` or `~/.bashrc` to automatically unlock on shell start (**only on a personal, encrypted workstation**):

```bash
# Unlock Bitwarden vault on shell start
# Requires BW_PASSWORD to be set via a secure mechanism (e.g., macOS Keychain)
if command -v bw &>/dev/null; then
  export BW_SESSION=$(bw unlock --passwordenv BW_PASSWORD --raw 2>/dev/null)
fi
```

> ⚠️ Only do this if your disk is encrypted and your workstation is single-user.

---

## Next Steps

- [Storing and retrieving secrets](usage.md)
- [Using Bitwarden in scripts](scripting.md)
