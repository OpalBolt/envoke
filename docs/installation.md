# Installation

## Requirements

- **Bitwarden CLI** (`bw`) — required for `bw://` references ([install guide](https://bitwarden.com/help/cli/))
- **Vault CLI** (`vault`) — required for `vault://` references ([install guide](https://developer.hashicorp.com/vault/docs/install))
- **kubectl** — optional; only needed to show the current context in `kctx status` output

## Install with Go

The simplest path if you have Go installed:

```bash
go install github.com/eficode/envoke/cmd/envoke@latest
```

This installs the `envoke` binary to `$GOPATH/bin` (typically `~/go/bin`). Make sure that directory is in your `PATH`:

```bash
# ~/.bashrc or ~/.zshrc
export PATH="$HOME/go/bin:$PATH"
```

## Build from source

```bash
git clone https://github.com/eficode/envoke.git
cd envoke
make build          # outputs to bin/envoke
```

Add `bin/` to your `PATH` or copy the binary to a directory already in `PATH`.

Requires Go 1.25+. See [go.dev/dl](https://go.dev/dl/) for installation.

## Shell integration (required)

After installing the binary, add one line to your shell config to enable auto-eval, background watcher, and EXIT trap:

**Bash / Zsh:**
```bash
eval "$(envoke shell-init)"
```

**Fish:**
```fish
envoke shell-init --shell fish | source
```

Add this to `~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish`. See [Setup](setup.md) for details.

---

## Install with Nix

If you use Nix flakes:

```bash
# Run directly without installing
nix run github:eficode/envoke

# Add to your system flake
inputs.envoke.url = "github:eficode/envoke";
```

### NixOS / Home Manager

```nix
# flake.nix (inputs)
inputs.envoke.url = "github:eficode/envoke";

# packages
environment.systemPackages = [ inputs.envoke.packages.${system}.envoke ];
```

### Development shell

```bash
nix develop    # enters dev shell with Go, goreleaser, bw, vault, and envoke
```

### Available Nix outputs

```bash
nix build              # builds envoke (default package)
nix run .#test         # run tests
nix run .#lint         # run linter
nix run .#fmt-check    # check formatting
```
