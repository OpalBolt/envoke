# kctx — keyless context

Fetch kubeconfig files from Vault or Bitwarden and write them to a RAM-backed tmpfile in `/dev/shm`. `KUBECONFIG` is set only in the current shell session and cleaned up automatically on exit. No credentials linger on disk.

---

## Introduction

`kctx switch` fetches a kubeconfig, writes it to `/dev/shm/kctx-<random>`, and exports `KUBECONFIG` pointing at it. A trap registers cleanup so the file is deleted when your shell exits.

```
╭─ kctx: prod ──────────────────────────────────────╮
│  KUBECONFIG   /dev/shm/kctx-a3f2b1                │
│  Context      prod-cluster                         │
│  Source       vault://secret/kubeconfig/prod       │
╰───────────────────────────────────────────────────╯
```

In a non-TTY context (pipes, scripts) the panel switches to compact plain-text automatically.

---

## Setup

### Shell integration (recommended)

Add **once** to your shell config:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(envoke shell-init)"
```

This defines the `kctx()` shell wrapper function and starts the background watcher (`kctx watch`). Completely silent when sourced.

The shell wrapper transparently handles the `switch` and `unload` commands by `eval`-ing the output (so `KUBECONFIG` changes take effect in the current shell). All other subcommands run the binary directly.

### Manual eval (without shell-init)

```bash
eval "$(kctx switch prod)"
eval "$(kctx unload)"
```

---

## Usage

### Switch context

```bash
kctx switch prod                          # fetch from Vault: secret/kubeconfig/prod
kctx switch prod secret/my-kubeconfig    # custom Vault path
kctx switch prod bw://kube/prod-config   # fetch from Bitwarden
```

### Unload

```bash
kctx unload   # unset KUBECONFIG and delete tmpfile
```

### Other commands

| Command | Description |
|---------|-------------|
| `kctx shell-init` | Emit shell wrapper and start watcher (silent) |
| `kctx status` | Show current `KUBECONFIG` path and active context |
| `kctx clear-cache` | Remove all Bitwarden cache files used by kctx |
| `kctx watch` | Background daemon for sleep/lock events (started by shell-init) |
| `kctx --version` | Print version |

---

## Secret sources

### Vault (default)

`kctx switch <env>` fetches from `vault://secret/kubeconfig/<env>` by default. Pass a custom path as the second argument:

```bash
kctx switch staging secret/infra/staging/kubeconfig
```

Requires `VAULT_ADDR` and `VAULT_TOKEN` to be set.

### Bitwarden

Provide a `bw://` URI as the second argument:

```bash
kctx switch prod bw://kube/prod-config
```

The kubeconfig content must be stored in the **Notes** field of the Bitwarden item. Custom fields have a character limit — always use Notes for kubeconfig files to avoid truncation.

### URI formats

| Format | Fetches |
|--------|---------|
| `vault://secret/path` | Vault KV v2 — `kubeconfig` field at path |
| `bw://folder/item` | Bitwarden — Notes field (default for kubeconfig) |
| `bw://folder/item/field:name` | Bitwarden — custom field named `name` |
| `bw://collection:name/item` | Bitwarden — item in a collection |

---

## Configuration

`kctx` shares the same config file as `renv`:

**Config file location:** `~/.config/renv/config.yaml` (respects `$XDG_CONFIG_HOME`)

### Environment variables

| Variable | Description |
|----------|-------------|
| `VAULT_ADDR` | Vault server URL |
| `VAULT_TOKEN` | Vault authentication token |
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive / CI) |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (non-interactive) |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) |
| `RENV_CACHE_MAX_AGE` | Bitwarden cache TTL (Go duration, e.g. `8h`) |
| `RENV_TIMEOUT_BITWARDEN` | Timeout for `bw` subprocess calls (e.g. `60s`) |
| `RENV_TIMEOUT_VAULT` | Timeout for `vault` subprocess calls (e.g. `60s`) |
| `RENV_LOG_LEVEL` | `debug` / `info` / `warn` / `error` |
| `RENV_LOG_FORMAT` | `text` (default) or `json` |

---

## Security model

| Property | Detail |
|----------|--------|
| RAM-backed tmpfiles | Written to `/dev/shm` (Linux tmpfs, cleared on reboot); `/tmp` fallback on macOS |
| AES-256 encrypted cache | Same cache model as `renv` — PBKDF2-SHA256 key derivation, 8-hour default TTL |
| Exit cleanup | Shell wrapper registers `trap 'kctx unload' EXIT` |
| No persistent kubeconfig | Tmpfile deleted on shell exit, lock, or sleep |

> ⚠️ **Known limitations:** root can read `/dev/shm`; macOS has no `/dev/shm` equivalent.

---

## Special integrations

### envoke resolve — loading kubeconfigs from .env

When using the `envoke resolve` command (the unified binary), you can declare kubeconfig directives alongside regular env secrets in your `.env` file:

```bash
# .env — safe to commit
DB_PASS=bw://prod/database
KCTX_PROD=bw://kubernetes/prod-cluster
KCTX_STAGING=vault://secret/kubeconfigs/staging
```

Keys prefixed with `KCTX_` are treated as kubeconfig directives rather than environment variables. `envoke resolve` fetches the kubeconfig and loads it into the `kctx` named store — `KCTX_PROD` becomes the context named `prod`, `KCTX_STAGING` becomes `staging`. The kubeconfig tmpfile and `KUBECONFIG` export are set up exactly as if you had run `kctx switch`.

### Sleep and screen-lock (Linux)

`kctx watch` listens for D-Bus signals from **systemd-logind**:

| Event | Action |
|-------|--------|
| Screen locked (`loginctl lock-session`) | Kubeconfig tmpfiles removed; `KUBECONFIG` unloaded from open shells; cache kept |
| System suspend / hibernate | Tmpfiles removed and Bitwarden cache cleared; full re-auth required on wake |

`kctx watch` is started automatically by `kctx shell-init`.

> **Note:** Locking directly via `swaylock` or `waylock` without `loginctl` does **not** emit the D-Bus signal. The tmpfile will remain until the shell exits.

For automatic locking with **swayidle**:

```bash
exec swayidle -w \
    timeout 300 'loginctl lock-session' \
    before-sleep 'loginctl lock-session'
```

### Nix shell

When `IN_NIX_SHELL` is set, `kctx` omits the `EXIT` trap from its output — Nix manages the shell lifecycle.
