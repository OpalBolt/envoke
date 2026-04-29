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

The config file is optional. Built-in defaults are sensible for most setups.

**Location:** `$XDG_CONFIG_HOME/envoke/config.yaml`  
(typically `~/.config/envoke/config.yaml` when `XDG_CONFIG_HOME` is unset)

**Override path:** `envoke --config /path/to/config.yaml <command>`

**Generate a commented default file:**
```bash
envoke config --init
```

### Config file reference

```yaml
log:
  level: warn        # debug | info | warn | error
  format: text       # text | json

cache:
  max_age: 8h        # TTL for cached Bitwarden folder data

timeouts:
  secrets: 30s       # timeout for bw CLI subprocess calls

ui:
  border: true       # show rounded box borders on summary panels
```

### Environment variable overrides

Environment variables override the config file. CLI flags override both.

| Variable | Description | Default |
|----------|-------------|---------|
| `ENVOKE_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `warn` |
| `ENVOKE_LOG_FORMAT` | Log format: `text` or `json` | `text` |
| `ENVOKE_CACHE_MAX_AGE` | Cache TTL (Go duration string, e.g. `8h`, `24h`) | `8h` |
| `ENVOKE_TIMEOUT_SECRETS` | Bitwarden CLI subprocess timeout | `30s` |
| `ENVOKE_UI_BORDER` | Show UI borders: `true` / `false` | `true` |
| `ENVOKE_BW_PASSWORD` | Bitwarden master password (skips interactive prompt) | — |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) | — |

### Precedence

```
CLI flags  >  ENVOKE_* env vars  >  config file  >  built-in defaults
```

---

## Bitwarden prerequisites

1. Install the Bitwarden CLI: `npm install -g @bitwarden/cli` (or your OS package manager)
2. Log in once: `bw login`
3. Vault items must be in **folders** (or collections) matching your `bw://` URIs

## Automation / CI

Supply credentials via environment variables to avoid interactive prompts:

```bash
export ENVOKE_BW_PASSWORD="your-master-password"
envoke resolve .env
```

Or provide a pre-existing session token to skip `bw unlock` entirely:

```bash
export BW_SESSION="$(bw unlock --raw)"
envoke resolve .env
```
