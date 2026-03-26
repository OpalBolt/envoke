# Shell Security — Avoiding Secrets in History and Logs

Shell history, process listings, and terminal logs are common places where secrets inadvertently appear.

---

## 1. Shell History

### Never type a secret directly into the shell prompt

```bash
# ❌ BAD — this will be saved to ~/.bash_history or ~/.zsh_history
export API_KEY=sk-abc123secret
curl -H "Authorization: Bearer sk-abc123secret" https://api.example.com

# ✅ GOOD — retrieve from Vault/Bitwarden programmatically
export API_KEY=$(vault kv get -field=key secret/myproject/api)
```

### Suppress specific commands from history

In Bash, prefix with a space (requires `HISTCONTROL=ignorespace`):

```bash
# ~/.bashrc
export HISTCONTROL=ignorespace:ignoredups
```

Then:

```bash
 export API_KEY=sk-abc123  # note the leading space — not saved to history
```

### Ignore patterns with HISTIGNORE

```bash
# ~/.bashrc or ~/.zshrc
export HISTIGNORE="export *KEY*:export *SECRET*:export *PASSWORD*:export *TOKEN*:*vault*login*"
```

### Zsh: setopt HIST_IGNORE_SPACE

```zsh
# ~/.zshrc
setopt HIST_IGNORE_SPACE
setopt HIST_NO_STORE     # don't store history commands themselves
```

---

## 2. Process List Exposure

When secrets are passed as CLI arguments, they appear in `ps` output:

```bash
# ❌ BAD — visible in `ps aux`
curl -u admin:mysecretpassword https://api.example.com
psql postgresql://user:mysecretpassword@host/db

# ✅ GOOD — use environment variables or files
PGPASSWORD=$(vault kv get -field=password secret/db) psql -U user -h host -d db

# ✅ GOOD — use stdin
echo "mysecretpassword" | command --password-stdin
```

---

## 3. Terminal Log Files

Some terminal multiplexers (tmux, screen) log sessions. Avoid running secret-handling commands in logged sessions, or configure logging to exclude sensitive data.

For tmux, disable logging by default:

```bash
# ~/.tmux.conf
# Do NOT enable automatic logging
# set -g @plugin 'tmux-plugins/tmux-logging'  # careful with this plugin
```

Clear scrollback buffer before working with secrets:

```bash
# tmux: clear history
tmux clear-history
# or Ctrl+L then clear scrollback: Prefix + Alt+l (if tmux-logging installed)
```

---

## 4. Debugging and Logging in Scripts

```bash
# ❌ BAD — bash -x prints every line including secret values
bash -x my-script.sh

# ✅ GOOD — selectively enable/disable tracing around secret handling
set +x   # disable trace
SECRET=$(vault kv get -field=key secret/api)
set -x   # re-enable trace
```

In application logs:

```bash
# ❌ BAD
echo "Connecting with password: $DB_PASSWORD"

# ✅ GOOD
echo "Connecting to database as $DB_USER"
```

---

## 5. Unset Secrets After Use

```bash
# Unset environment variables when no longer needed
use_secret() {
  local secret
  secret=$(vault kv get -field=password secret/db)
  
  # Use the secret...
  psql -U app -h db.example.com -d mydb <<< "SELECT 1"
  
  # Unset immediately after use
  unset secret
}
```

Use a `trap` to ensure cleanup:

```bash
cleanup() {
  unset DB_PASSWORD
  unset API_KEY
  bw lock >/dev/null 2>&1
}
trap cleanup EXIT INT TERM
```

---

## 6. Secure Terminal Habits

| Practice | Implementation |
|---|---|
| Auto-lock screen | `pmset displaysleeptime 5` (macOS) or `xset s 300` (Linux) |
| Clear clipboard after use | `pbpaste | head -c 0 | pbcopy` (macOS) / `xclip -i /dev/null` (Linux) |
| Use a password manager keyboard shortcut | Bitwarden: `Ctrl+Shift+L` to auto-fill |
| Lock Bitwarden vault before stepping away | `bw lock` |
| Review shell history periodically | `history | grep -i "key\|secret\|password\|token"` |

---

## Related

- [Git security](git-security.md)
- [.env file security](env-files.md)
