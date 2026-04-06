# kctx — Ephemeral Kubeconfig Switcher

`kctx` fetches kubeconfig files from Vault or Bitwarden and writes them to a
RAM-backed tmpfile in `/dev/shm` (or `/tmp`). `KUBECONFIG` is set only in the
current shell session and is cleaned up automatically on exit. No credentials
linger on disk.

## Installation

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/kctx@latest
```

Or via Nix:

```bash
nix profile install github:eficode/secure-handling-of-secrets#kctx
```

## Shell setup (recommended)

Add **once** to your shell config:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(kctx shell-init)"
```

`kctx shell-init` is completely **silent** when sourced — it defines a shell
wrapper function and starts the background watcher (`kctx watch`) without
printing anything to the terminal.

### How the shell wrapper works

`kctx shell-init` emits a shell function that transparently wraps the binary.
`switch` and `unload` output shell statements (`export KUBECONFIG=…` /
`unset KUBECONFIG`) that are `eval`'d so they take effect in the current shell.
All other subcommands (`status`, `clear-cache`, …) run the binary directly:

```bash
kctx() {
  case "$1" in
    unload)
      eval "$(command kctx unload)"
      ;;
    status)
      command kctx status
      ;;
    *)
      eval "$(command kctx "$@")"
      ;;
  esac
}
```

## Commands

| Command | Description |
|---------|-------------|
| `kctx shell-init` | Emit shell wrapper function + start watcher (silent) |
| `kctx switch <env> [source]` | Fetch kubeconfig and set `KUBECONFIG` in current shell |
| `kctx unload` | Unset `KUBECONFIG` and remove tmpfile |
| `kctx status` | Show current `KUBECONFIG` path, managed/external, and current context |
| `kctx clear-cache` | Remove all Bitwarden cache files used by kctx |
| `kctx watch` | Background daemon for sleep/lock events (started by shell-init) |
| `kctx --version` | Print version |

## Usage

```bash
kctx switch prod                          # fetch from Vault: secret/kubeconfig/prod
kctx switch prod secret/my-kubeconfig    # custom Vault path
kctx switch prod bw://kube/prod-config   # fetch from Bitwarden

kctx unload                              # unset KUBECONFIG and remove tmpfile
kctx status                              # show current KUBECONFIG path and context
kctx clear-cache                         # remove all kctx cache files
```

## Output

After a successful `kctx switch`, a styled panel is printed to **stderr** showing
what was loaded:

On a TTY (rounded-border box, colored via [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)):

```
╭─ kctx: prod ──────────────────────────────────────╮
│  KUBECONFIG   /dev/shm/kctx-a3f2b1                │
│  Context      prod-cluster                         │
│  Source       vault://secret/kubeconfig/prod       │
╰───────────────────────────────────────────────────╯
```

On non-TTY stderr (pipes, scripts — compact plain-text):

```
kctx: prod
  KUBECONFIG  /dev/shm/kctx-a3f2b1
  Context     prod-cluster
  Source      vault://secret/kubeconfig/prod
```

The `Context` field shows the current kubectl context name, queried with
`KUBECONFIG` set to the new tmpfile immediately after writing it.

**stdout** contains the shell statements to eval:

```
export KUBECONFIG=/dev/shm/kctx-a3f2b1
trap 'kctx unload' EXIT
```

## Sources

### Vault (default)

By default `kctx switch <env>` fetches from `vault://secret/kubeconfig/<env>`.
Pass a custom path as the second argument:

```bash
kctx switch staging secret/infra/staging/kubeconfig
```

Requires `VAULT_ADDR` and `VAULT_TOKEN` to be set.

### Bitwarden

Provide a `bw://` URI as the second argument:

```bash
kctx switch prod bw://kube/prod-config
```

The kubeconfig content must be stored in the **Notes** field of the Bitwarden
item. Bitwarden custom fields have a character limit — always use Notes for
kubeconfig files to avoid truncation.

The `bw://` format is `bw://<folder>/<item-name>` (fetches the Notes field by
default). See the URI format table for all options.

### URI formats

| Scheme | Format | Fetches |
|--------|--------|---------|
| Vault KV v2 | `vault://secret/path` | `kubeconfig` field at the path |
| Bitwarden (notes) | `bw://folder/item` | Notes field (default for kubeconfig) |
| Bitwarden (custom) | `bw://folder/item/field:name` | Custom field |
| Bitwarden (collection) | `bw://collection:name/item` | Item in a Bitwarden collection |

## Security model

| Property | Detail |
|----------|--------|
| RAM-backed tmpfiles | Written to `/dev/shm` (Linux tmpfs, cleared on reboot); `/tmp` fallback |
| AES-256 encrypted Bitwarden cache | Same cache model as `renv` — PBKDF2-SHA256 key derivation, 8-hour TTL |
| Exit cleanup | Shell wrapper registers `trap 'kctx unload' EXIT` |
| No persistent kubeconfig | Tmpfile is deleted on shell exit, lock, or sleep |

⚠️ **Known limitations:** root can read `/dev/shm`; macOS has no `/dev/shm` equivalent.

## Sleep and screen-lock integration (Linux)

`kctx watch` listens for D-Bus signals from **systemd-logind**:

| Event | D-Bus signal | Action |
|-------|-------------|--------|
| Screen locked | `org.freedesktop.login1.Session.Lock` | Removes kubeconfig tmpfiles; `KUBECONFIG` unloaded from open shells. Cache kept. |
| System suspend/hibernate | `org.freedesktop.login1.Manager.PrepareForSleep` | Removes tmpfiles and clears Bitwarden cache. Full re-auth required after wake. |

`kctx watch` is started automatically by `kctx shell-init`.

### Requirement: lock via loginctl / D-Bus

The lock signal only reaches `kctx` when the session is locked through
`loginctl lock-session`:

```bash
loginctl lock-session
```

> **Note:** Invoking a screen locker directly (e.g. `swaylock`, `waylock`)
> without `loginctl` does **not** emit the D-Bus signal. `kctx` will not
> receive the lock event and the kubeconfig tmpfile will remain until the
> shell exits.

## Environment variables

| Variable | Description |
|----------|-------------|
| `VAULT_ADDR` | Vault server URL |
| `VAULT_TOKEN` | Vault authentication token |
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive / CI) |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (non-interactive) |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) |
| `RENV_CACHE_MAX_AGE` | Bitwarden cache TTL (Go duration, e.g. `8h`) |
| `RENV_TIMEOUT_BITWARDEN` | Timeout for `bw` subprocess calls (Go duration, e.g. `60s`) |
| `RENV_TIMEOUT_VAULT` | Timeout for `vault` subprocess calls (Go duration, e.g. `60s`) |
| `RENV_LOG_LEVEL` | `debug` / `info` / `warn` / `error` |
| `RENV_LOG_FORMAT` | `text` (default) or `json` |
