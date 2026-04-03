# kctx — Ephemeral Kubeconfig Switcher

`kctx` fetches kubeconfig files from Vault or Bitwarden and writes them to a RAM-backed
tmpfile (`/dev/shm` or `/tmp`). `KUBECONFIG` is set only in the current shell session and
cleaned up automatically on exit.

## Installation

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/kctx@latest
```

## Shell integration

The `kctx switch` and `kctx unload` commands emit shell statements that must be `eval`'d so
that environment variables are set in the **parent shell** rather than a subprocess.
The `shell-init` command generates a wrapper function that handles this automatically.

Add to your shell config (`~/.bashrc` or `~/.zshrc`):

```bash
eval "$(kctx shell-init)"
```

This defines a `kctx` shell function that transparently wraps the binary. After sourcing,
you use `kctx` as normal — no manual `eval` required.

### How the shell wrapper works

`kctx shell-init` emits a shell function like this:

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
      eval "$(command kctx switch "$@")"
      ;;
  esac
}
```

`switch` and `unload` output shell statements (`export KUBECONFIG=…` / `unset KUBECONFIG`)
that are `eval`'d so they take effect in the current shell. All other subcommands
(`status`, `clear-cache`, …) run the binary directly.

## Usage

```bash
kctx switch prod                          # fetch from Vault: secret/kubeconfig/prod
kctx switch prod secret/my-kubeconfig    # custom Vault path
kctx switch prod bw://kube/prod-config   # fetch from Bitwarden (see below)
kctx unload                              # unset KUBECONFIG and remove tmpfile
kctx status                              # show current KUBECONFIG path
kctx clear-cache                         # remove all kctx cache files
kctx --version                           # print version
```

`kctx switch` automatically registers `trap 'kctx unload' EXIT` so the kubeconfig is
removed when the shell exits.

## Bitwarden

When using Bitwarden as the secret backend, the kubeconfig content must be stored in the
**Notes** field of a Bitwarden item. The `bw://` reference format is:

```
bw://<folder>/<item-name>
```

Example: a Bitwarden item named `prod-config` in folder `kube` with the kubeconfig YAML
pasted into the Notes field is referenced as:

```bash
kctx switch prod bw://kube/prod-config
```

> **Note:** Bitwarden's custom fields have a character limit. Always use the **Notes** field
> for kubeconfig files to avoid truncation.

## Vault

The default Vault path convention is `secret/kubeconfig/<env>` with field `kubeconfig`.
Pass a custom path as the second argument to override:

```bash
kctx switch staging secret/infra/staging/kubeconfig
```
