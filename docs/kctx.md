# kctx — Ephemeral Kubeconfig Switcher

`kctx` fetches kubeconfig files from Vault or Bitwarden and stores them in RAM.

## Installation

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/kctx@latest
```

## Shell integration

```bash
source <(kctx-bin shell-init)
```

## Usage

```bash
kctx switch prod                          # vault://secret/kubeconfig/prod#kubeconfig
kctx switch prod secret/my-kubeconfig    # custom Vault path
kctx switch prod bw://kube/prod-config   # from Bitwarden
kctx clear                               # unset KUBECONFIG, remove tmpfile
kctx status                              # show current context
```

The `kctx` shell function wraps `kctx-bin` and runs `eval` so `KUBECONFIG` is set in the parent shell.
