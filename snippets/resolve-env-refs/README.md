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
| `bw` CLI | `bw://` references |
| `jq` | `bw://` custom field extraction |
| `vault` CLI | `vault://` references |
| `VAULT_ADDR` env var | `vault://` references |
| `VAULT_TOKEN` env var | `vault://` references |

**Bitwarden session:** `BW_SESSION` is obtained automatically. To avoid interactive prompts:
- Set `BW_PASSWORD` to unlock non-interactively
- Set `BW_CLIENTID` + `BW_CLIENTSECRET` for API key login

---

## Cleanup

Resolved secrets are tracked in `_LOADED_ENV_VARS` and unset:
- **Automatically** when the shell exits (via `trap unload_env EXIT`)
- **When you leave** a direnv-managed directory (`cd` away)
- **Manually** via `unload_env`

YAML mode: secrets exist only in memory (stdout pipe) or in a `/dev/shm` temp file that is deleted after the command exits.

---

## Security notes

- Never `eval "$(resolve-env-refs.sh .env)"` — use `source <(...)` instead. `eval` re-interprets secret values as shell code.
- Keys are validated against `[A-Za-z_][A-Za-z0-9_]*` before emission to prevent injection attacks.
- YAML resolved values are always double-quoted and escaped, preventing YAML injection from secrets containing special characters.
- `resolve_yaml_exec` uses `/dev/shm` (RAM) when available so secrets are never written to a physical disk.
