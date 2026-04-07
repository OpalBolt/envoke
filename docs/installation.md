# Installation

## Requirements

- [Bitwarden CLI (`bw`)](https://bitwarden.com/help/cli/) — required for `bw://` secret references
- [HashiCorp Vault CLI (`vault`)](https://developer.hashicorp.com/vault/docs/install) — required for `vault://` secret references

You only need the backends you intend to use.

---

## Go

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/envoke@latest
```

This installs the `envoke` binary. The `renv` and `kctx` commands are provided as shell functions by `envoke shell-init` — see [Shell setup](#shell-setup) below.

Requires Go 1.22 or later.

### Shell setup

Add **once** to your shell config:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(envoke shell-init)"

# fish: ~/.config/fish/config.fish
envoke shell-init --shell fish | source
```

This defines `renv()` and `kctx()` shell wrapper functions, and starts the background watcher daemon. It is completely **silent** — nothing is printed when your shell starts.

After setup, use `renv` and `kctx` as normal commands:

```bash
renv resolve .env
kctx switch prod
```

---

## Nix

### Flake (ad-hoc)

```bash
nix profile install github:eficode/envoke
```

### Flake input (NixOS / home-manager)

```nix
inputs.envoke.url = "github:eficode/envoke";

# Then add to environment.systemPackages or home.packages:
inputs.envoke.packages.${system}.envoke
```

When installed via Nix, the shell setup step is the same — add `eval "$(envoke shell-init)"` to your shell config.

---

## Build from source

```bash
git clone https://github.com/eficode/secure-handling-of-secrets
cd secure-handling-of-secrets
make build          # outputs to bin/
```

Requires `nix develop` (or Go 1.22+ with `CGO_ENABLED=0`) in your environment.
