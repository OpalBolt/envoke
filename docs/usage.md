# Usage

## envoke resolve

Resolves a `.env` file, exporting secrets as shell variables and writing context files to secure tmpfiles.

```bash
eval "$(envoke resolve)"          # resolves .env in current directory
eval "$(envoke resolve prod.env)" # resolves a specific file
```

With [shell-init](install.md#shell-integration) active, the `eval` is handled automatically.

**Secret references** are resolved from Bitwarden (or other configured providers) and exported as env vars:

```bash
DB_PASSWORD=bw://myapp/db
API_KEY=bw://myapp/api/field:key
```

**CTX_ entries** write a secret to a tmpfile in `/dev/shm` and export an env var pointing at it. The `#ENVVAR` fragment determines which env var to set — envoke does not care what the file contains or what tool reads it:

```bash
CTX_PROD=bw://k8s/prod#KUBECONFIG
CTX_PROD=bw://talos/prod#TALOSCONFIG
CTX_META=bw://aws/shared#AWS_SHARED_CREDENTIALS_FILE
CTX_META=bw://tokens/github/password#GITHUB_TOKEN
```

`CTX_META` entries form a persistent baseline that is re-applied on every `envoke switch`. Any other `CTX_<GROUP>` name is a switchable group.

**ENVOKE_DEFAULT_GROUP** auto-switches to a group after resolve, so no second `eval` is needed:

```bash
ENVOKE_DEFAULT_GROUP=prod
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--file`, `-f` | Path to .env file | `.env` |
| `--shell` | Shell type for trap generation: `bash`, `zsh`, `fish` | `bash` |
| `--force` | Bypass terminal check and print exports to terminal | `false` |

---

## envoke switch

Switches to a named context group. All env vars from the previous group are unset, the META baseline is re-applied, and the new group's env vars are exported — pointing at already-written tmpfiles. No provider calls are made.

```bash
eval "$(envoke switch prod)"
eval "$(envoke switch staging)"
```

`envoke switch meta` is rejected — META is always active and cannot be switched to directly.

With shell-init active, the `eval` is handled automatically.

---

## envoke status

Shows loaded env vars, context groups, and the active variable summary.

```bash
envoke status
```

---

## envoke unload

Unsets all variables exported by `envoke resolve` and removes all context tmpfiles.

```bash
eval "$(envoke unload)"
```

With shell-init active, the `eval` is handled automatically.

---

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

---

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

---

## envoke clear-cache

Removes the stored Bitwarden session and all managed tmpfiles from the secure cache directory (`/run/user/<uid>`, `/dev/shm`, or `/tmp`).

```bash
envoke clear-cache
```

---

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

---

## envoke config

Shows configuration documentation. Use `--init` to write a commented default config file.

```bash
envoke config
envoke config --init            # write ~/.config/envoke/config.yaml
envoke config --init --force    # overwrite existing config file
```

See [Configuration](config.md) for the full config reference.

---

## envoke watch

Background daemon that watches for screen lock and sleep events. Normally started automatically by shell-init — you do not need to run this manually.

On lock: secret variables are unloaded from open shells and context tmpfiles are removed. (Linux only — screen lock detection is not implemented on macOS. See [Known limitations](limitations.md).)
On sleep: all caches are cleared, requiring full re-authentication after wake.

---

## envoke load *(deprecated)*

`envoke load` has been removed. Use `CTX_` entries in your `.env` file instead:

```bash
# Before
envoke load prod bw://kubernetes/prod-cluster

# After — add to .env
CTX_PROD=bw://kubernetes/prod#KUBECONFIG
```

Then `eval "$(envoke resolve .env)"` and `eval "$(envoke switch prod)"`.
