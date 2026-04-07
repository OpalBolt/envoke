# Usage

## envoke

`envoke` is the unified binary. Use it to load secrets and kubeconfigs in one command.

### envoke resolve

Resolves a `.env` file, loading secrets as shell exports and kubeconfigs into named stores.

```bash
envoke resolve           # resolves .env in current directory
envoke resolve secrets.env
```

**Example `.env`:**
```bash
# Plain values pass through unchanged
DB_HOST=postgres.internal

# Secret references are resolved at load time
DB_PASSWORD=bw://database/prod-db
API_TOKEN=vault://secret/api#token

# KCTX_ lines load kubeconfigs into named stores (not exported as env vars)
KCTX_PROD=bw://kubernetes/prod-cluster
KCTX_STAGING=vault://secret/kubeconfig/staging#kubeconfig
```

With shell-init active, no explicit `eval` is needed:
```bash
envoke resolve .env
# Secrets are now in your environment
# kctx prod / kctx staging are ready to use
```

Without shell-init:
```bash
eval "$(envoke resolve .env)"
```

### envoke unload

Unsets all variables that were loaded by the last `envoke resolve`, and removes any managed kubeconfig temp files.

```bash
envoke unload
```

### envoke clear-cache

Removes all encrypted cache files from `/dev/shm` (or `/tmp`) for the current user.

```bash
envoke clear-cache
```

### envoke shell-init

Prints the shell integration snippet. Add to your shell config — see [Setup](setup.md).

```bash
envoke shell-init            # bash/zsh
envoke shell-init --shell fish
```

### envoke watch

Background daemon for lock/sleep detection. Started automatically by shell-init. You do not normally need to run this manually.

---

## renv *(remote env)*

`renv` handles environment secret resolution. All renv commands are available as `envoke renv <subcommand>`.

### renv resolve

Resolves secret references in a `.env` file and emits shell exports.

```bash
envoke renv resolve           # resolves .env
envoke renv resolve prod.env  # resolves a specific file
```

Output (evaluated by the shell function):
```bash
export DB_PASSWORD='s3cr3t'
export API_TOKEN='ghp_abc123'
```

### renv exec

Runs a command with the resolved environment injected, without modifying the current shell.

```bash
envoke renv exec --env secrets.env -- myapp --config prod
envoke renv exec -- python manage.py migrate
```

The command replaces the `renv` process (`exec` syscall). The current shell environment is not affected.

### renv yaml

Resolves secret references inside a YAML file.

```bash
envoke renv yaml config.yaml
envoke renv yaml config.yaml --key database.password
```

Any `bw://` or `vault://` values found anywhere in the YAML are resolved inline. Use `--key` to extract a single value using dot notation.

### renv unload

Unsets all variables that were tracked by the last `renv resolve`.

```bash
envoke renv unload
```

### renv status

Shows which variables are currently loaded and the state of the cache.

```bash
envoke renv status
```

Output includes:
- Tracked variable names
- Cache file locations with ages (colour-coded: green < 30 min, yellow < 4 h, red older)

### renv clear-cache

Removes encrypted Bitwarden cache files for the current user.

```bash
envoke renv clear-cache
```

---

## kctx *(Keyless ConTeXt)*

`kctx` manages named contexts fetched from Bitwarden or Vault — currently focused on kubeconfigs, with support for additional context types planned. All kctx commands are available as `envoke kctx <subcommand>`.

### kctx load

Fetches a kubeconfig and stores it encrypted in `/dev/shm` under a local name.

```bash
envoke kctx load prod bw://kubernetes/prod-cluster
envoke kctx load staging vault://secret/kubeconfig/staging#kubeconfig
envoke kctx load dev bw://collection:k8s/dev-cluster
```

The name (`prod`, `staging`, `dev`) is how you'll refer to it in `kctx switch`. Names must match `[a-zA-Z0-9._-]+`.

### kctx switch

Decrypts a named kubeconfig and sets `KUBECONFIG` in your shell.

```bash
envoke kctx switch prod
envoke kctx switch staging

# Shorthand (with shell-init active):
kctx prod
kctx staging
```

If shell-init is active, the shorthand `kctx <name>` expands to `kctx switch <name>` automatically.

You can also switch and fetch on-the-fly without pre-loading:
```bash
kctx switch prod bw://kubernetes/prod-cluster
```

### kctx unload

Removes the managed kubeconfig temp file and unsets `KUBECONFIG`.

```bash
envoke kctx unload
```

### kctx status

Shows the current `KUBECONFIG`, the active kubectl context, and all named kubeconfigs in the local store.

```bash
envoke kctx status
```

### kctx clear-cache

Removes all encrypted kubeconfig store files and managed temp files.

```bash
envoke kctx clear-cache
```

---

## .env file reference

### Syntax

```bash
# Comments start with #
KEY=value                   # bare value
KEY="value with spaces"     # double-quoted
KEY='value with spaces'     # single-quoted
export KEY=value            # export prefix is accepted and stripped
```

### Bitwarden reference format

```bash
KEY=bw://folder/item                    # password field (default)
KEY=bw://folder/item/username           # username field
KEY=bw://folder/item/note               # notes/secure note field
KEY=bw://folder/item/totp               # TOTP code
KEY=bw://folder/item/field:custom_name  # custom field by name
KEY=bw://collection:name/item           # item in a named collection
```

### Vault reference format

```bash
KEY=vault://secret/path#field           # KV v2; field fragment is required
```

`VAULT_ADDR` and `VAULT_TOKEN` must be set in the environment.

### Kubeconfig directives (envoke only)

```bash
KCTX_NAME=bw://folder/item              # loaded into kctx named store as "name"
KCTX_NAME=vault://secret/path#field     # same, from Vault
```

The `KCTX_` prefix is stripped and the remainder is lowercased to form the store name: `KCTX_PROD` → `prod`.

---

## Flags

All commands accept these global flags:

| Flag | Description |
|------|-------------|
| `--verbose` | Enable debug logging |
| `--log-level LEVEL` | Set log level: `debug`, `info`, `warn`, `error` |
| `--no-cache` | Disable the encrypted cache (prompts Bitwarden every time) |
| `--config PATH` | Path to config file |

---

## Cache behaviour

Bitwarden folder data is cached encrypted in `/dev/shm` (or `/tmp` as fallback). The default TTL is 8 hours, configurable via `RENV_CACHE_MAX_AGE`.

Within the TTL, only your **local password** is prompted — Bitwarden is not contacted. After the TTL expires, or after `clear-cache`, both passwords are prompted again.

Use `--no-cache` to disable caching entirely and fetch from Bitwarden on every invocation.

---

## Automatic cleanup

When shell-init is active, the following events trigger automatic cleanup:

| Event | Action |
|-------|--------|
| Shell exit | Unload secrets, clear cache, kill watcher |
| Screen lock | Unset loaded variables in open shells |
| System sleep | Clear cache and session (full re-auth on wake) |

On Linux, lock and sleep detection uses D-Bus (requires systemd-logind). On macOS, sleep detection uses timer drift; screen lock requires a launchd agent (not bundled). On Windows, automatic detection is not yet implemented.
