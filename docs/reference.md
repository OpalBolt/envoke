# Command reference

## envoke resolve

Resolves a `.env` file, exporting secrets as shell variables and loading kubeconfig directives into the named store.

```bash
envoke resolve           # resolves .env in current directory
envoke resolve prod.env  # resolves a specific file
```

Output must be evaluated by your shell. With shell-init active this is automatic:

```bash
envoke resolve .env
# secrets are now in your environment
# KCTX_* entries are loaded; use envoke switch <name> to activate one
```

Without shell-init:

```bash
eval "$(envoke resolve .env)"
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--file`, `-f` | Path to .env file | `.env` |
| `--shell` | Shell type for trap generation: `bash`, `zsh`, `fish` | `bash` |
| `--force` | Bypass terminal check and print exports to terminal | `false` |

## envoke exec

Runs a command with the resolved environment injected. No `eval` required.

```bash
envoke exec -- myprogram --flag value
envoke exec --env secrets.env -- python manage.py migrate
```

The resolved variables are injected into the subprocess environment only — the current shell is not modified.

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--env`, `-e` | Path to .env file | `.env` |

## envoke yaml

Resolves `bw://` references inside a YAML file and prints the result.

```bash
envoke yaml config.yaml
envoke yaml config.yaml --key database.password   # extract a single value
```

## envoke load

Fetches a kubeconfig from Bitwarden and caches it under a local name.

```bash
envoke load prod bw://kubernetes/prod-cluster
envoke load dev bw://collection:k8s/dev-cluster
```

Names must match `[a-zA-Z0-9._-]+`.

## envoke switch

Activates a named kubeconfig by setting `KUBECONFIG` in your shell.

```bash
envoke switch prod
envoke switch staging bw://k8s/staging   # fetch on the fly if not pre-loaded
```

With shell-init active, the shell function handles the `eval` automatically.

## envoke unload

Unsets all variables exported by `envoke resolve` and clears `KUBECONFIG` if it was set by envoke.

```bash
envoke unload
```

Output must be evaluated. With shell-init active this is automatic.

## envoke status

Shows tracked env vars, current `KUBECONFIG`, and named kubeconfigs in the local store.

```bash
envoke status
```

## envoke clear-cache

Removes the stored Bitwarden session and all kubeconfig files from `/dev/shm` (or `/tmp`).

```bash
envoke clear-cache
```

## envoke shell-init

Prints the shell integration snippet. Add to your shell config — see [Installation](install.md#shell-integration).

```bash
envoke shell-init              # bash/zsh
envoke shell-init --shell fish
```

## envoke config

Shows configuration documentation. Use `--init` to write a commented default config file.

```bash
envoke config
envoke config --init            # write ~/.config/envoke/config.yaml
envoke config --init --force    # overwrite existing config file
```

See [Configuration](install.md#configuration) for the full config reference.

## envoke watch

Background daemon that watches for screen lock and sleep events. Normally started automatically by shell-init — you do not need to run this manually.

On lock: secret variables are unloaded from open shells and managed kubeconfig tempfiles are removed.  
On sleep: all caches are cleared, requiring full re-authentication after wake.

---

## .env file syntax

```bash
# Comments start with #
KEY=value
KEY="value with spaces"
KEY='value with spaces'
export KEY=value        # export prefix is accepted and stripped
```

### Bitwarden reference format

```bash
KEY=bw://folder/item                    # password field (default)
KEY=bw://folder/item/username           # username field
KEY=bw://folder/item/note               # notes / secure note
KEY=bw://folder/item/totp               # TOTP code
KEY=bw://folder/item/field:custom_name  # custom field by name
KEY=bw://collection:name/item           # item in a named collection
```

### Kubeconfig directives

Lines prefixed with `KCTX_` load a kubeconfig into the named store rather than being exported as env vars:

```bash
KCTX_PROD=bw://kubernetes/prod-cluster
KCTX_STAGING=bw://kubernetes/staging-cluster
```

The prefix is stripped and the remainder is lowercased to form the store name: `KCTX_PROD` → `prod`.

---

## Global flags

All commands accept these flags:

| Flag | Description |
|------|-------------|
| `--verbose` | Enable debug logging (shorthand for `--log-level=debug`) |
| `--log-level LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `--config PATH` | Path to config file |

---

## Cache behaviour

Bitwarden folder data is cached encrypted in `/dev/shm` (RAM-backed tmpfs, falls back to `/tmp`). The default TTL is 8 hours, configurable via `ENVOKE_CACHE_MAX_AGE` or the config file.

Within the TTL, only your local password is prompted — Bitwarden is not contacted. After the TTL or after `clear-cache`, both passwords are required again.

Use `--no-cache` to disable caching entirely.

---

## Automatic cleanup

When shell-init is active:

| Event | Action |
|-------|--------|
| Shell exit | Unload secrets, clear cache, kill watcher |
| Screen lock | Unset loaded variables in open shells |
| System sleep | Clear cache — full re-auth required on wake |

On Linux, lock/sleep detection uses D-Bus (systemd-logind). On macOS, sleep detection uses a timer-drift heuristic; screen lock detection is not implemented. See [Known limitations](limitations.md).
