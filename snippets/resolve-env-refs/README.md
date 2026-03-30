# resolve-env-refs

Resolve `bw://` and `vault://` secret references in `.env` and YAML files — without leaving secrets on disk.

## Install

```bash
git clone https://github.com/eficode/secure-handling-of-secrets
cd secure-handling-of-secrets
./snippets/resolve-env-refs/install.sh
```

This copies `resolve-env-refs.sh` to `~/.config/resolve-env-refs/resolve-env-refs.sh`.

**To update:** pull the repo and run `./snippets/resolve-env-refs/install.sh` again. It detects whether an update is needed by comparing SHA-256 hashes.

**To check if up to date:**

```bash
./snippets/resolve-env-refs/install.sh --check
```

---

## .env / .envrc usage

First, make the functions available in your shell:

```bash
source ~/.config/resolve-env-refs/resolve-env-refs.sh
```

### Pattern 1 — direnv (recommended)

[direnv](https://direnv.net/) loads the env when you enter the directory and **unloads it automatically when you leave** — no cleanup needed.

```bash
# .envrc
source ~/.config/resolve-env-refs/resolve-env-refs.sh
source <(resolve_env_file .env)
```

```bash
direnv allow .   # grant permission once per project
```

### Pattern 2 — self-loading .env (bash and zsh)

Put this as the **first line** of your `.env`. When you `source .env`, it:
1. Loads the resolver from `~/.config`
2. Detects bash or zsh automatically
3. Unloads any previously active env (prints a message when switching projects)
4. Resolves all `bw://` and `vault://` references and exports them
5. Returns early so the raw reference strings are never executed as shell assignments
6. Registers `unload_env()` + `trap unload_env EXIT` for cleanup

```bash
# .env
source ~/.config/resolve-env-refs/resolve-env-refs.sh \
  && declare -f _load_self_env &>/dev/null \
  && _load_self_env \
  && return 0 2>/dev/null; true

# References below are parsed by resolve_env_file — not executed by bash/zsh:
DATABASE_URL=bw://prod-db/password
DATABASE_USER=bw://prod-db/username
STRIPE_KEY=bw://stripe-api/field:api_key
VAULT_TOKEN=vault://secret/myproject/app#token
```

```bash
source .env        # resolves all refs, registers EXIT trap
unload_env         # optional: manual cleanup before shell exits
echo "$_LOADED_ENV_FILE"   # shows which env is currently active
```

> ⚠️ **Trap chaining**: `resolve_env_file` installs `trap unload_env EXIT`, replacing any existing EXIT trap.
> To chain: `trap 'unload_env; your_existing_cleanup' EXIT` after sourcing.

### Pattern 3 — exec mode (safest: secrets never enter your shell)

```bash
# Resolved values are injected directly into the child process only:
resolve-env-refs.sh .env -- node server.js
resolve-env-refs.sh .env -- python app.py

# Source into current shell (safe; never use eval):
source <(resolve-env-refs.sh .env)
```

---

## YAML usage

Resolves `bw://` and `vault://` references in YAML scalar values. Supports unquoted, single-quoted, and double-quoted values. Resolved values are always emitted as double-quoted YAML strings.

### Stream mode — pipe resolved YAML directly to a tool

Secrets go to stdout only. Nothing is written to disk.

```bash
source ~/.config/resolve-env-refs/resolve-env-refs.sh

# Helm
resolve_yaml_file values.yaml | helm upgrade myapp . -f -

# kubectl
resolve_yaml_file config.yaml | kubectl apply -f -

# Any tool that reads YAML from stdin
resolve_yaml_file secrets.yaml | tool --config -
```

### Exec mode — for tools that require a file path

A temporary file is created in `/dev/shm` (RAM-backed) when available, or in `/tmp` with `chmod 600`. The `{}` placeholder in any argument is replaced with the temp file path. The file is deleted immediately after the command exits.

```bash
# {} is replaced with the secure temp file path
resolve_yaml_exec values.yaml -- helm upgrade myapp . -f {}
resolve_yaml_exec config.yaml -- kubectl apply -f {}
resolve_yaml_exec secrets.yaml -- some-tool --config {}
```

### Bash scripts — extract individual values

For bash scripts that need specific config values rather than piping the full YAML, use `resolve_yaml_value`. It resolves the whole file, then extracts the key. Requires **yq** or **python3+pyyaml**.

```bash
source ~/.config/resolve-env-refs/resolve-env-refs.sh

# Extract resolved values into bash variables
DB_HOST=$(resolve_yaml_value config.yaml database.host)
DB_PASS=$(resolve_yaml_value config.yaml database.password)   # resolves bw://
API_KEY=$(resolve_yaml_value config.yaml api.stripe_key)      # resolves vault://

echo "Connecting to $DB_HOST"
./my-app --db-host "$DB_HOST" --db-pass "$DB_PASS"
```

Dot notation traverses nested keys; integer segments index lists:

```bash
FIRST_URL=$(resolve_yaml_value services.yaml services.0.url)
```

---

## Python drop-in

`resolve_yaml_refs.py` is a zero-dependency-except-pyyaml module that recursively resolves all `bw://` and `vault://` references in any YAML structure. Drop it next to your script or install it in your project.

```bash
pip install pyyaml
cp snippets/resolve-env-refs/resolve_yaml_refs.py your-project/
```

### Import usage

```python
from resolve_yaml_refs import load_yaml

# All refs resolved — returns a plain Python dict.
config = load_yaml("config.yaml")
db_password = config["database"]["password"]    # actual secret
api_key     = config["api"]["stripe_key"]       # actual secret

# From an already-loaded string:
from resolve_yaml_refs import load_yaml_string
config = load_yaml_string(yaml_text)

# Resolve a single reference:
from resolve_yaml_refs import resolve_value
secret = resolve_value("bw://prod-db/password")
secret = resolve_value("vault://secret/myapp#token")
```

Works with any nesting depth and with lists:

```yaml
# config.yaml
database:
  password: bw://prod-db/password
  username: bw://prod-db/username
services:
  - name: stripe
    api_key: vault://secret/payments#stripe_key
  - name: sendgrid
    api_key: bw://sendgrid/field:api_key
```

```python
config = load_yaml("config.yaml")
# All four refs above are resolved — nested dicts and lists handled automatically.
stripe_key = config["services"][0]["api_key"]
```

### CLI usage

```bash
# Resolved YAML to stdout (pipe anywhere):
python resolve_yaml_refs.py config.yaml
python resolve_yaml_refs.py values.yaml | helm upgrade myapp . -f -

# Extract a single value:
python resolve_yaml_refs.py config.yaml --key database.password
DB_PASS=$(python resolve_yaml_refs.py config.yaml --key database.password)
```

### YAML reference syntax

```yaml
# Supported — scalar values (unquoted, single-quoted, or double-quoted):
database:
  password: bw://prod-db/password
  username: bw://prod-db/username
api:
  stripe_key: "bw://stripe-api/field:api_key"
  vault_secret: 'vault://secret/myproject/app#token'
```

> ⚠️ `vault://path` without `#field` is **not supported** in YAML mode — it would expand to
> multiple key/value pairs, which cannot map to a single YAML scalar. Use `vault://path#field`.

---

## Reference syntax

| Reference | Retrieves |
|-----------|-----------|
| `bw://item-name` | Bitwarden item password (default) |
| `bw://item-name/password` | Bitwarden password field |
| `bw://item-name/username` | Bitwarden username field |
| `bw://item-name/note` | Bitwarden notes field |
| `bw://item-name/field:fname` | Bitwarden custom field named `fname` |
| `vault://secret/path#field` | Vault KV v2 field at path |
| `vault://secret/path` | All Vault KV v2 fields (.env mode only) |

---

## Prerequisites

| Tool | Required for |
|------|-------------|
| `bw` CLI | `bw://` references (bash + Python) |
| `jq` | `bw://` custom field extraction (bash only) |
| `vault` CLI | `vault://` references (bash + Python) |
| `VAULT_ADDR` env var | `vault://` references |
| `VAULT_TOKEN` env var | `vault://` references |
| `pyyaml` | Python drop-in (`pip install pyyaml`) |
| `yq` or `python3+pyyaml` | `resolve_yaml_value` (bash) |

**Bitwarden session:** `BW_SESSION` is required for both bash and Python.
- Bash: set automatically on first unlock; or set `BW_PASSWORD` for non-interactive unlock
- Python: must be set before calling `load_yaml`: `export BW_SESSION=$(bw unlock --raw)`

---

## Cleanup

Resolved secrets are tracked in `_LOADED_ENV_VARS` and unset:
- **Automatically** when the shell exits (via `trap unload_env EXIT`)
- **When you leave** a direnv-managed directory (`cd` away)
- **Manually** via `unload_env`

YAML mode: secrets exist only in memory (stdout pipe) or in a `/dev/shm` temp file that is deleted after the command exits. The Python module never writes resolved values to disk.

---

## Security notes

- Never `eval "$(resolve-env-refs.sh .env)"` — use `source <(...)` instead. `eval` re-interprets secret values as shell code.
- Keys are validated against `[A-Za-z_][A-Za-z0-9_]*` before emission to prevent injection attacks.
- YAML resolved values are always double-quoted and escaped, preventing YAML injection from secrets containing special characters.
- `resolve_yaml_exec` uses `/dev/shm` (RAM) when available so secrets are never written to a physical disk.
- The Python module calls `bw` and `vault` CLIs via subprocess with `capture_output=True` — resolved values are never echoed to a terminal or log.
