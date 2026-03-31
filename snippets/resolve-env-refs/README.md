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

DATABASE_URL=bw://prod-db/password
DATABASE_USER=bw://prod-db/username
STRIPE_KEY=bw://stripe-api/field:api_key
VAULT_TOKEN=vault://secret/myproject/app#token
```

```bash
source .env        # resolves all refs, registers EXIT trap
unload_env         # optional: manual cleanup
```

> **Trap chaining**: `resolve_env_file` registers `trap unload_env EXIT`. To chain:
> `trap 'unload_env; your_cleanup' EXIT` after sourcing.

---

## Reference syntax

| Reference | Retrieves |
|-----------|-----------|
| `bw://item-name` | Bitwarden password field (default) |
| `bw://item-name/password` | Bitwarden password field |
| `bw://item-name/username` | Bitwarden username field |
| `bw://item-name/note` | Bitwarden notes field |
| `bw://item-name/field:fname` | Bitwarden custom field `fname` |
| `vault://secret/path#field` | Vault KV v2 field |

---

## Prerequisites

| Tool | Required for |
|------|-------------|
| `bw` + `jq` | `bw://` references |
| `vault` | `vault://` references (`VAULT_ADDR` + `VAULT_TOKEN` must be set) |

**Bitwarden session**: `BW_SESSION` must be set before sourcing. If not set, the script will print instructions:

```
resolve-env-refs: BW_SESSION is not set.
  Run:  export BW_SESSION=$(bw unlock --raw)
  If items are missing, also run:  bw sync
```

The script checks `bw status` before the (slow) `bw list items` call, so you get a clear error if the vault is locked or you are not logged in.

---

## Security

- `bw list items` is called **once** and cached in `$_RENV_BW_ITEMS` for the shell session — subsequent refs in the same `.env` are free.
- All tracked vars are unset on shell EXIT via `trap unload_env EXIT`.
- Use `source <(resolve_env_file .env)` — never `eval "$(…)"`.
- Variable names are validated against `[A-Za-z_][A-Za-z0-9_]*` before emission.

