# resolve-env-refs

Resolve `bw://` and `vault://` secret references in `.env` files — secrets land in memory only, never on disk.

## Install

```bash
./snippets/resolve-env-refs/install.sh
```

Copies `resolve-env-refs.sh` to `~/.config/resolve-env-refs/resolve-env-refs.sh`.

```bash
./snippets/resolve-env-refs/install.sh --check   # is it up to date?
./snippets/resolve-env-refs/install.sh            # install or upgrade
```

---

## Usage

### Pattern 1 — direnv (recommended)

direnv auto-loads on `cd` and unloads when you leave.

```bash
# .envrc
source ~/.config/resolve-env-refs/resolve-env-refs.sh
source <(resolve_env_file .env)
```

```bash
direnv allow .
```

### Pattern 2 — self-loading .env

Put this as the **first line** of your `.env`. Running `source .env` resolves all refs and registers an EXIT trap for cleanup. The raw reference strings are never executed as shell assignments.

```bash
# .env
source ~/.config/resolve-env-refs/resolve-env-refs.sh \
  && source <(resolve_env_file "${BASH_SOURCE[0]:-$0}") \
  && return 0 2>/dev/null; true

DATABASE_URL=bw://production/prod-db/password
DATABASE_USER=bw://production/prod-db/username
STRIPE_KEY=bw://payments/stripe-api/field:api_key
VAULT_TOKEN=vault://secret/myproject/app#token
```

```bash
source .env        # resolves all refs, registers EXIT trap
unload_env         # optional: manual cleanup
renv_clear_cache   # optional: remove on-disk cache files
```

> **Trap chaining**: `resolve_env_file` registers `trap unload_env EXIT`. To chain:
> `trap 'unload_env; your_cleanup' EXIT` after sourcing.

---

## Reference syntax

| Reference | Retrieves |
|-----------|-----------|
| `bw://folder/item-name` | Bitwarden password field (default) |
| `bw://folder/item-name/password` | Bitwarden password field |
| `bw://folder/item-name/username` | Bitwarden username field |
| `bw://folder/item-name/note` | Bitwarden notes field |
| `bw://folder/item-name/field:fname` | Bitwarden custom field `fname` |
| `vault://secret/path#field` | Vault KV v2 field |

The **folder** segment is required for `bw://` references. It scopes the Bitwarden fetch to only items in that folder.

---

## Prerequisites

| Tool | Required for |
|------|-------------|
| `bw` + `jq` | `bw://` references |
| `openssl` | AES-256 cache encryption |
| `vault` | `vault://` references (`VAULT_ADDR` + `VAULT_TOKEN` must be set) |

**Bitwarden login**: You must be logged in before sourcing (`bw login`).

**Authentication**: On first use the script prompts for your Bitwarden master password, or reads it from `RENV_BW_PASSWORD`:

```bash
# Non-interactive (CI/CD, direnv without a terminal):
export RENV_BW_PASSWORD=<master-password>
source <(resolve_env_file .env)
```

The master password serves two purposes:
1. Unlock the vault: `bw unlock --passwordenv`
2. Derive the AES-256 key for the on-RAM folder cache

After all folder data is fetched, the `BW_SESSION` token is cleared from memory and the environment. Future shells decrypt the cache using only the master password — no Bitwarden network call needed.

---

## Security

### Folder-scoped RAM cache

Each Bitwarden folder referenced in a `.env` gets its own encrypted cache file in `/dev/shm` (Linux tmpfs RAM). Only the items in that folder are fetched and stored — not the entire vault.

The cache file is:
- Named by the first 16 hex chars of `SHA-256(uid:folder_name)` — per-user, per-folder, no collisions
- Encrypted with AES-256-CBC + PBKDF2, key derived from `SHA-256(master_password)`
- `chmod 600` — inaccessible to other users
- Discarded automatically after 8 hours (`_RENV_CACHE_MAX_AGE`)
- **Persists across shells** — next shell decrypts with password, no BW API call
- Manually removable: `renv_clear_cache`

### Session token lifecycle

| Phase | BW_SESSION |
|-------|-----------|
| Before `resolve_env_file` | Prompted for password / `RENV_BW_PASSWORD` read |
| During folder fetch (cache miss) | Session token live in `_RENV_BW_SESSION` |
| After all folders loaded | `BW_SESSION` **cleared** from env and memory |
| Subsequent shells (cache hit) | Never needed — cache decrypted with master password |

### What the encryption actually protects

| Threat | Protected? |
|--------|-----------|
| Other non-root users reading `/dev/shm` | ✅ file permissions + encryption |
| Attacker without master password who reads the file | ✅ AES-256 ciphertext is useless without the key |
| Root | ❌ root can read `/dev/shm` and `/proc/<pid>/environ` |
| Kernel swap | ❌ decrypted JSON lives in bash variables; those pages can be swapped to disk if swap is enabled (inherent bash limitation) |
| SIGKILL / OOM kill | ❌ EXIT trap does not fire; encrypted file stays in RAM until reboot (harmless — it is encrypted and `/dev/shm` clears on reboot) |
| macOS | ⚠️ no `/dev/shm`; falls back to `/tmp` which is **disk-backed** |

### Other security properties

- `bw list items --folderid` is called **once per folder** and cached — all subsequent refs to the same folder are free.
- `BW_SESSION` is cleared from memory and the environment after all folders are fetched.
- All tracked vars are unset on shell EXIT via `trap unload_env EXIT`.
- Use `source <(resolve_env_file .env)` — never `eval "$(…)"`.
- Variable names are validated against `[A-Za-z_][A-Za-z0-9_]*` before emission.

### Migration from old format

The `bw://` reference format changed in this version:

| Old (deprecated) | New |
|-----------------|-----|
| `bw://item-name` | `bw://folder/item-name` |
| `bw://item-name/field` | `bw://folder/item-name/field` |

Update your `.env` files by adding the Bitwarden folder name as the first path segment. Old-format refs produce a clear error message with migration guidance.

