# Setup

## Shell integration

The shell integration wraps `envoke` in a shell function that automatically evaluates the output of `resolve` and `unload` commands, so you don't need to manually `eval` anything after the initial setup.

### Bash / Zsh

Add to `~/.bashrc` or `~/.zshrc`:

```bash
eval "$(envoke shell-init)"
```

### Fish

Add to `~/.config/fish/config.fish`:

```fish
envoke shell-init --shell fish | source
```

### What shell-init does

- Defines `envoke()`, `renv()`, and `kctx()` shell functions (all backed by the single `envoke` binary)
- Starts a background watcher (`envoke watch`) that detects screen lock and system sleep
- Installs a `PROMPT_COMMAND` / `precmd` hook that checks for unload signals
- Installs an EXIT trap that unloads secrets, clears the cache, and kills the watcher

The watcher removes secrets from your shell on screen lock and clears the cache on sleep, so no manual cleanup is needed.

## Configuration file

The config file is optional. Defaults are sensible for most use cases.

**Location:** `$XDG_CONFIG_HOME/renv/config.yaml` (typically `~/.config/renv/config.yaml`)

```yaml
log:
  level: warn        # debug | info | warn | error
  format: text       # text | json

cache:
  max_age: 8h        # how long Bitwarden folder data is cached

timeouts:
  bitwarden: 30s     # timeout for bw CLI calls
  vault: 30s         # timeout for vault CLI calls

ui:
  border: true       # show rounded box borders in output panels
```

## Environment variables

Environment variables override the config file. CLI flags override both.

| Variable | Description | Default |
|----------|-------------|---------|
| `RENV_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `warn` |
| `RENV_LOG_FORMAT` | Log format: `text` or `json` | `text` |
| `RENV_CACHE_MAX_AGE` | Cache TTL (Go duration, e.g. `8h`, `24h`) | `8h` |
| `RENV_TIMEOUT_BITWARDEN` | `bw` CLI timeout | `30s` |
| `RENV_TIMEOUT_VAULT` | `vault` CLI timeout | `30s` |
| `RENV_UI_BORDER` | Show UI borders: `true`/`false` | `true` |
| `RENV_BW_PASSWORD` | Bitwarden master password (skips interactive prompt) | — |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (skips interactive prompt) | — |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) | — |
| `VAULT_ADDR` | Vault server address | — |
| `VAULT_TOKEN` | Vault authentication token | — |

## Bitwarden prerequisites

1. Install the Bitwarden CLI: `npm install -g @bitwarden/cli` or your OS package manager
2. Log in: `bw login`
3. Your vault items must be organized in **folders** (or collections) matching your `bw://` URIs

## Vault prerequisites

1. Install the Vault CLI: see [developer.hashicorp.com/vault](https://developer.hashicorp.com/vault/docs/install)
2. Set `VAULT_ADDR` and `VAULT_TOKEN` before running envoke
3. Secrets must be stored as KV v2 entries

## Automation / CI

In non-interactive environments, supply passwords via environment variables to avoid TTY prompts:

```bash
export RENV_BW_PASSWORD="your-bitwarden-master-password"
export RENV_LOCAL_PASSWORD="your-local-cache-password"
export VAULT_ADDR="https://vault.example.com"
export VAULT_TOKEN="hvs.CAESIB..."

envoke resolve .env
```

Or use `BW_SESSION` to provide a pre-existing Bitwarden session token and skip the unlock step entirely.
