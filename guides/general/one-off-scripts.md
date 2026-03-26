# Keys for One-Off Scripts

**Scenario:** You need to write a quick script to call an API — update a record, trigger a webhook, rotate some data. You have a key. Where does it go?

This is the most common situation consultants run into, and also the most common source of accidentally committed credentials.

---

## The Options (best → worst)

| Option | Friction | Risk | Recommended? |
|---|---|---|---|
| Pull from Bitwarden at runtime | Low | None — key never touches disk or history | ✅ Yes |
| Shell env var (set before running) | Low | Low — if you follow the rules below | ✅ Yes |
| `.env` file | Medium | Medium — must never be committed | ⚠️ With care |
| Hardcoded in the script | None | High — will end up in git | ❌ Never |
| Passed as a CLI argument | None | High — visible in `ps aux` and history | ❌ Never |

---

## Option 1: Pull from Bitwarden at Runtime (Recommended)

Store the key in Bitwarden as a login item or custom field. Retrieve it inline — the key is never written to disk, never exported to your shell environment, never in history.

```bash
#!/usr/bin/env bash
# update-record.sh

set -euo pipefail

# Pull the key at runtime — not stored anywhere
API_KEY=$(bw get password "my-api-key-item")

curl -s \
  -X PATCH "https://api.example.com/records/123" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'

unset API_KEY
```

**First run — unlock Bitwarden once per session:**

```bash
export BW_SESSION=$(bw unlock --raw)
# Now run your script — bw commands will work without prompting
bash update-record.sh
```

See [`snippets/bw-get-secret.sh`](../../snippets/bw-get-secret.sh) for helper functions like `bw_ensure_unlocked` that handle the session automatically.

---

## Option 2: Shell Environment Variable

Set the key in your current shell session, run the script, then unset it. The key lives in memory only and is never written to the script file.

**Set without saving to history** (prefix with a space — requires `HISTCONTROL=ignorespace`):

```bash
#  export API_KEY=sk-abc123   # leading space — not saved to ~/.bash_history
```

**Or retrieve from Bitwarden/Vault for the session:**

```bash
export API_KEY=$(bw get password "my-api-key-item")
bash update-record.sh
unset API_KEY
```

**In the script — read from the environment, fail loudly if missing:**

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${API_KEY:?API_KEY is not set. Run: export API_KEY=\$(bw get password \"my-api-key-item\")}"

curl -s \
  -X PATCH "https://api.example.com/records/123" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'
```

The `: "${VAR:?message}"` pattern exits immediately with a clear error if the variable is missing — better than a silent 401.

---

## Option 3: .env File (Use With Care)

If you must use a `.env` file, follow these rules without exception:

1. **Add `.env` to `.gitignore` before creating the file**
2. Never send the file over Slack, email, or any chat tool
3. Delete the file when the script is no longer needed

```bash
# .gitignore — add this BEFORE creating .env
.env
*.env
```

```bash
# .env  — never commit this
API_KEY=sk-abc123
API_BASE_URL=https://api.example.com
```

Load and run in a single command so the values don't persist in your shell:

```bash
# Load .env only for the duration of this command
env $(grep -v '^#' .env | xargs) bash update-record.sh

# Or with dotenv-cli
dotenv -- bash update-record.sh
```

```bash
# Python — load from .env using python-dotenv
pip install python-dotenv
```

```python
# update_record.py
import os
import httpx
from dotenv import load_dotenv

load_dotenv()  # loads .env if present, falls back to real env vars

api_key = os.environ["API_KEY"]  # raises KeyError if missing — intentional
base_url = os.environ["API_BASE_URL"]

response = httpx.patch(
    f"{base_url}/records/123",
    headers={"Authorization": f"Bearer {api_key}"},
    json={"status": "active"},
)
response.raise_for_status()
print(response.json())
```

---

## Cleanup Pattern (Bash)

Use `trap` to ensure the key is always unset, even if the script fails:

```bash
#!/usr/bin/env bash
set -euo pipefail

API_KEY=$(bw get password "my-api-key-item")
trap 'unset API_KEY' EXIT INT TERM

# ... do work ...
```

---

## What Not To Do

```bash
# ❌ Hardcoded in the script — will end up in git
curl -H "Authorization: Bearer sk-abc123secret" https://api.example.com

# ❌ Passed as a CLI argument — visible in `ps aux`
./update-record.sh sk-abc123secret

# ❌ Echoed into the script from the shell — ends up in history
echo "API_KEY=sk-abc123" >> config.sh

# ❌ Committed .env file — even in a "private" repo
git add .env && git commit -m "add config"
```

---

## Quick Reference

```bash
# Store the key in Bitwarden (one time)
bw create item  # or use the Bitwarden desktop app

# Unlock Bitwarden for the session
export BW_SESSION=$(bw unlock --raw)

# Run your script with the key pulled at runtime
API_KEY=$(bw get password "item-name") bash update-record.sh

# Or export for the session and unset after
export API_KEY=$(bw get password "item-name")
bash update-record.sh
unset API_KEY
```

---

## Related

- [bw-get-secret.sh snippet](../../snippets/bw-get-secret.sh)
- [inject-env.sh snippet](../../snippets/inject-env.sh)
- [Bitwarden setup guide](../bitwarden/setup.md)
- [Bitwarden scripting guide](../bitwarden/scripting.md)
- [.env file security](env-files.md)
- [Shell security](shell-security.md)
