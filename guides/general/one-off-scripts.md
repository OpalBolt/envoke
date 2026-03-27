# Keys for One-Off Scripts

**Scenario:** You need to write a quick script to call an API — update a record, trigger a webhook, rotate some data. You have a key. Where does it go?

This is the most common situation consultants run into, and also the most common source of accidentally committed credentials.

---

## The Options (best → worst)

| Option | Friction | Risk | Recommended? |
|---|---|---|---|
| Pull from Bitwarden via snippet | Low | None — key never touches disk or history | ✅ Yes |
| Pull from Bitwarden directly with `bw` | Low | None | ✅ Yes |
| Shell env var (set before running) | Low | Low — if you follow the rules below | ✅ Yes |
| `.env` file | Medium | Medium — must never be committed | ⚠️ With care |
| Hardcoded in the script | None | High — will end up in git | ❌ Never |
| Passed as a CLI argument | None | High — visible in `ps aux` and history | ❌ Never |

---

## Option 1: Pull from Bitwarden at Runtime (Recommended)

Store the key in Bitwarden once. Retrieve it at runtime — the key is never written to disk, never in shell history.

### Using the bw CLI directly (recommended for bash scripts)

```bash
#!/usr/bin/env bash
set -euo pipefail

# Unlock once per session (no-op if BW_SESSION is already set)
if [[ -z "${BW_SESSION:-}" ]]; then
  BW_SESSION=$(bw unlock --raw)
  export BW_SESSION
fi

# Retrieve the key — password field by default
API_KEY=$(bw get password "my-api-key-item" --session "$BW_SESSION")
trap 'unset API_KEY' EXIT INT TERM

curl -s \
  -X PATCH "https://api.example.com/records/123" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'
```

Available `bw get` sub-commands:

| Command | What it retrieves |
|---------|-------------------|
| `bw get password "item"` | The password field of a login item |
| `bw get username "item"` | The username field |
| `bw get notes "item"` | The notes field (certificates, private keys) |
| `bw get item "item" \| jq -r '.fields[] \| select(.name == "fname") \| .value'` | A named custom field |

### Using `bw` directly (when you can't source the snippet)

If you're writing a standalone script that can't reference the snippet path:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Unlock once per session — skip if BW_SESSION is already set
if [[ -z "${BW_SESSION:-}" ]]; then
  BW_SESSION=$(bw unlock --raw) || { echo "❌ Failed to unlock Bitwarden" >&2; exit 1; }
  export BW_SESSION
fi

API_KEY=$(bw get password "my-api-key-item" --session "$BW_SESSION") \
  || { echo "❌ Could not retrieve API key from Bitwarden" >&2; exit 1; }
trap 'unset API_KEY' EXIT INT TERM

curl -s \
  -X PATCH "https://api.example.com/records/123" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'
```

---

## Option 2: Shell Environment Variable

Set the key in your current shell session, run the script, then unset it. Useful when the script itself shouldn't know anything about Bitwarden.

```bash
# Retrieve from Bitwarden into your shell, then run the script
export API_KEY=$(bw get password "my-api-key-item" --session "$BW_SESSION")
bash update-record.sh
unset API_KEY
```

**In the script — read from the environment, fail loudly if missing:**

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${API_KEY:?API_KEY is not set. Export it first: export API_KEY=\$(bw get password \"item-name\" --session \"\$BW_SESSION\")}"

curl -s \
  -X PATCH "https://api.example.com/records/123" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}'
```

The `: "${VAR:?message}"` pattern exits immediately with a clear error if the variable is missing — better than a silent 401.

---

## Option 3: .env File with References (Use With Care)

Never put actual secret values in a `.env` file. Instead, use references that are resolved at runtime:

```bash
# .env.example — safe to commit (references, not real secrets)
API_KEY=bw://my-api-key-item/password
API_BASE_URL=https://api.example.com
```

Resolve references before running your command:

```bash
# Resolve and inject into a child process (recommended)
./snippets/resolve-env-refs.sh .env.example -- bash update-record.sh

# Or resolve into the current shell (safe — use source, not eval)
source <(./snippets/resolve-env-refs.sh .env.example)
bash update-record.sh
unset API_KEY
```

If you must use a `.env` file with literal values (legacy), follow these rules without exception:

1. **Add `.env` to `.gitignore` before creating the file**
2. Never send the `.env` file over Slack, email, or chat
3. Delete the file when the script is no longer needed

```bash
# .gitignore — add this BEFORE creating .env
.env
*.env
```

Load for the duration of the command only:

```bash
# Bash — scoped to this one command
env $(grep -v '^#' .env | xargs) bash update-record.sh

# Or with resolve-env-refs.sh exec mode (bw:// and vault:// refs supported)
./snippets/resolve-env-refs.sh .env -- bash update-record.sh
```

---

## Complete Examples

### Bash

A fully self-contained script using the `bw` CLI directly. Copy, adjust the item name and endpoint, run.

```bash
#!/usr/bin/env bash
# update-record.sh — patch a record at an API endpoint
#
# Requires: bw (Bitwarden CLI), jq
# Usage:    bash update-record.sh
#
# Store your API key in Bitwarden as a login item named "my-api-key-item".
# The script will prompt to unlock the vault once per session.

set -euo pipefail

# Unlock once per session
if [[ -z "${BW_SESSION:-}" ]]; then
  echo "🔐 Unlocking Bitwarden vault..." >&2
  BW_SESSION=$(bw unlock --raw) || { echo "❌ Failed to unlock Bitwarden vault" >&2; exit 1; }
  export BW_SESSION
fi

# --- Configuration ---
BW_ITEM="my-api-key-item"        # Name of the Bitwarden item
API_BASE="https://api.example.com"
RECORD_ID="123"

# --- Retrieve secret ---
API_KEY=$(bw get password "$BW_ITEM" --session "$BW_SESSION")
trap 'unset API_KEY' EXIT INT TERM

# --- Do the work ---
response=$(curl -s -w "\n%{http_code}" \
  -X PATCH "${API_BASE}/records/${RECORD_ID}" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"status": "active"}')

body=$(echo "$response" | head -n -1)
status=$(echo "$response" | tail -n 1)

if [[ "$status" -ge 200 && "$status" -lt 300 ]]; then
  echo "✅ Updated (HTTP ${status}): $(echo "$body" | jq -c '.' 2>/dev/null || echo "$body")"
else
  echo "❌ Request failed (HTTP ${status}): $body" >&2
  exit 1
fi
```

### Python

Two patterns depending on how you want to supply the key.

**Pattern A — inject via environment (cleanest, no Bitwarden dependency in the script):**

```bash
# Run it — key comes from Bitwarden, script reads from env
export BW_SESSION=$(bw unlock --raw)
API_KEY=$(bw get password "my-api-key-item" --session "$BW_SESSION") \
  python update_record.py
```

```python
#!/usr/bin/env python3
# update_record.py — patch a record at an API endpoint
#
# Requires: httpx  (pip install httpx)
# Usage:    API_KEY=<key> python update_record.py
#           Or use resolve-env-refs.sh / export from bw before running.

import os
import sys
import httpx

API_BASE = "https://api.example.com"
RECORD_ID = "123"

# Fail fast with a clear message — don't let a missing key produce a silent 401
api_key = os.environ.get("API_KEY")
if not api_key:
    sys.exit(
        "❌ API_KEY is not set.\n"
        "   Export it first:\n"
        "     export BW_SESSION=$(bw unlock --raw)\n"
        '     export API_KEY=$(bw get password "my-api-key-item" --session "$BW_SESSION")'
    )

try:
    response = httpx.patch(
        f"{API_BASE}/records/{RECORD_ID}",
        headers={"Authorization": f"Bearer {api_key}"},
        json={"status": "active"},
        timeout=10,
    )
    response.raise_for_status()
    print(f"✅ Updated (HTTP {response.status_code}):", response.json())
except httpx.HTTPStatusError as e:
    sys.exit(f"❌ Request failed (HTTP {e.response.status_code}): {e.response.text}")
except httpx.RequestError as e:
    sys.exit(f"❌ Network error: {e}")
```

**Pattern B — call `bw` from within the script (no pre-export needed):**

```python
#!/usr/bin/env python3
# update_record.py — fetches its own key from Bitwarden via subprocess
#
# Requires: httpx  (pip install httpx)
# Usage:    python update_record.py
#           Vault will prompt once if BW_SESSION is not already set.

import os
import sys
import subprocess
import httpx

API_BASE = "https://api.example.com"
RECORD_ID = "123"
BW_ITEM = "my-api-key-item"


def bw_get_password(item_name: str) -> str:
    session = os.environ.get("BW_SESSION", "")
    cmd = ["bw", "get", "password", item_name]
    if session:
        cmd += ["--session", session]

    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        sys.exit(f"❌ Could not retrieve '{item_name}' from Bitwarden: {result.stderr.strip()}")
    return result.stdout.strip()


api_key = bw_get_password(BW_ITEM)

try:
    response = httpx.patch(
        f"{API_BASE}/records/{RECORD_ID}",
        headers={"Authorization": f"Bearer {api_key}"},
        json={"status": "active"},
        timeout=10,
    )
    response.raise_for_status()
    print(f"✅ Updated (HTTP {response.status_code}):", response.json())
except httpx.HTTPStatusError as e:
    sys.exit(f"❌ Request failed (HTTP {e.response.status_code}): {e.response.text}")
except httpx.RequestError as e:
    sys.exit(f"❌ Network error: {e}")
finally:
    del api_key  # clear from memory
```

> **Which pattern?** Use Pattern A when the script is simple or you control how it's launched. Use Pattern B when the script needs to be fully self-contained and you don't want to manage exports manually.

---

## What Not To Do

```bash
# ❌ Hardcoded in the script — will end up in git
curl -H "Authorization: Bearer sk-abc123secret" https://api.example.com

# ❌ Passed as a CLI argument — visible in `ps aux` and shell history
./update-record.sh sk-abc123secret

# ❌ Echoed into a config file
echo "API_KEY=sk-abc123" >> config.sh

# ❌ Committed .env file — even in a "private" repo
git add .env && git commit -m "add config"

# ❌ Real secret value in .env.example — defeats the purpose
echo "API_KEY=sk-abc123real" > .env.example  # NO — use bw://item-name/field instead
```

---

## Quick Reference

```bash
# Unlock Bitwarden for the session (once)
export BW_SESSION=$(bw unlock --raw)

# Option A: call bw directly
API_KEY=$(bw get password "my-api-key-item" --session "$BW_SESSION")

# Option B: inject into a script without touching your shell (exec mode)
./snippets/resolve-env-refs.sh .env -- bash update-record.sh
./snippets/resolve-env-refs.sh .env -- python update_record.py

# Option C: .env with references (team-friendly, safe to commit)
#   .env contains: API_KEY=bw://my-api-key-item/password
./snippets/resolve-env-refs.sh .env -- bash update-record.sh

# Option D: load into current shell (direnv or source .env)
source <(./snippets/resolve-env-refs.sh .env)
```

---

## Related

- [resolve-env-refs.sh snippet](../../snippets/resolve-env-refs.sh)
- [Bitwarden setup guide](../bitwarden/setup.md)
- [Bitwarden scripting guide](../bitwarden/scripting.md)
- [.env file security](env-files.md)
- [Shell security](shell-security.md)
