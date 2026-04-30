# Usage

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

Runs a command with the resolved environment injected. No `eval` required — suitable for CI and scripts.

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

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--key` | Dot-notation key to extract (e.g. `database.password`) | — |

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

Output must be evaluated. With shell-init active this is automatic:

```bash
envoke switch prod
```

Without shell-init:

```bash
eval "$(envoke switch prod)"
```

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

Removes the stored Bitwarden session and all kubeconfig files from the secure cache directory (`/run/user/<uid>`, `/dev/shm`, or `/tmp`).

```bash
envoke clear-cache
```

## envoke shell-init

Prints the shell integration snippet. Add to your shell config — see [Installation](install.md#shell-integration).

```bash
envoke shell-init              # bash/zsh
envoke shell-init --shell fish
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--shell` | Shell type: `bash`, `zsh`, `fish` | `bash` |
| `--force` | Bypass terminal check (for scripting) | `false` |

## envoke config

Shows configuration documentation. Use `--init` to write a commented default config file.

```bash
envoke config
envoke config --init            # write ~/.config/envoke/config.yaml
envoke config --init --force    # overwrite existing config file
```

See [Configuration](config.md) for the full config reference.

## envoke watch

Background daemon that watches for screen lock and sleep events. Normally started automatically by shell-init — you do not need to run this manually.

On lock: secret variables are unloaded from open shells and managed kubeconfig tempfiles are removed. (Linux only — screen lock detection is not implemented on macOS. See [Known limitations](limitations.md).)  
On sleep: all caches are cleared, requiring full re-authentication after wake.
