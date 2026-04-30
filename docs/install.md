# Installation & configuration

## Requirements

- **Bitwarden CLI** (`bw`) — required for `bw://` references ([install guide](https://bitwarden.com/help/cli/))
- **kubectl** — optional; only needed to show the active context in `envoke status` output

## Install with Go

```bash
go install github.com/opalbolt/envoke/cmd/envoke@latest
```

The binary is placed in `$GOPATH/bin` (typically `~/go/bin`). Make sure that directory is in your `PATH`.

## Install with Nix

```bash
# Run directly without installing
nix run github:opalbolt/envoke

# Add to your system flake
inputs.envoke.url = "github:opalbolt/envoke";
```

See [Nix integration](nix.md) for NixOS / Home Manager setup.

## Build from source

```bash
git clone https://github.com/opalbolt/envoke.git
cd envoke
make build          # outputs to bin/envoke
```

Requires Go 1.25+.

## Shell integration

Add one line to your shell config to enable auto-eval, background watcher, and EXIT trap:

**Bash / Zsh** (`~/.bashrc` or `~/.zshrc`):
```bash
eval "$(envoke shell-init)"
```

**Fish** (`~/.config/fish/config.fish`):
```fish
envoke shell-init --shell fish | source
```

Once active, `envoke resolve`, `envoke switch`, and `envoke unload` all work without a manual `eval`.

The shell-init snippet:
- Wraps `envoke` in a shell function that auto-evals `resolve`, `unload`, and `switch` output
- Starts a background watcher (`envoke watch`) for lock/sleep detection
- Installs an EXIT trap that unloads secrets, clears the cache, and kills the watcher

---

## Configuration

Configuration is optional. For details on all config options, environment variables, credential handling, and examples, see [Configuration](config.md).

Quick start:

```bash
# Generate commented default config
envoke config --init

# Override config at runtime
envoke --config /path/to/config.yaml resolve .env
```

**Environment variable precedence:**  
CLI flags > ENVOKE_* env vars > config file > built-in defaults

---

## Bitwarden prerequisites

1. Install the Bitwarden CLI: `npm install -g @bitwarden/cli` (or your OS package manager)
2. Log in once: `bw login`
3. Vault items must be in **folders** (or collections) matching your `bw://` URIs

## Automation / CI

For headless/CI environments, supply credentials via environment variables:

```bash
export ENVOKE_BW_PASSWORD="your-master-password"
envoke resolve .env
```

Or use a pre-existing session token:

```bash
export BW_SESSION="$(bw unlock --raw)"
envoke resolve .env
```

For detailed configuration options and examples, see [Configuration](config.md).
