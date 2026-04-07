# renv — remote env

Resolve `bw://` and `vault://` secret references in `.env` and YAML files. Secrets are fetched at runtime and injected into your shell or a subprocess. Nothing is written to disk in plaintext — secrets live only in process memory or a short-lived AES-256 encrypted cache in `/dev/shm`.

---

## Introduction

Instead of putting actual secrets in your config files, you use reference URIs:

```bash
# .env — safe to commit
APP_ENV=production
DB_PASS=bw://prod/database
API_KEY=vault://secret/myapp#api_key
```

`renv resolve` fetches the values at runtime and either injects them into your shell or passes them to a subprocess. Literal values (like `APP_ENV` above) are passed through unchanged.

Output is printed to **stderr** as a styled panel showing what was loaded and where it came from:

```
╭─ renv: loaded .env ─────────────────────────────╮
│  DB_PASS   bw://prod/database                   │
│  API_KEY   vault://secret/myapp#api_key          │
│  APP_ENV   (literal)                             │
╰─────────────────────────────────────────────────╯
```

In a non-TTY context (direnv, pipes) the panel switches to compact plain-text automatically.

---

## Setup

### Shell integration (recommended)

Add **once** to your shell config:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(envoke shell-init)"

# fish: ~/.config/fish/config.fish
envoke shell-init --shell fish | source
```

This defines the `renv()` shell function so that `renv resolve` and `renv unload` modify the current shell without manual `eval`. It also starts the background watcher (`renv watch`). Completely silent when sourced.

### Manual eval (without shell-init)

If you prefer not to use `shell-init`:

```bash
eval "$(renv resolve .env)"
eval "$(renv unload)"
```

---

## Usage

### Resolve a `.env` file

```bash
renv resolve           # resolves .env in current directory
renv resolve prod.env  # resolves a specific file
```

### Unload secrets

```bash
renv unload   # unsets all variables loaded by renv
```

### Run a command with secrets injected

`renv exec` resolves the `.env` and runs the command with secrets in its environment. The current shell is **not** modified — ideal for scripts, CI, and one-off commands:

```bash
renv exec -- docker compose up
renv exec -- pytest --verbose
renv exec --env staging.env -- ./deploy.sh
```

### Resolve a YAML file

```bash
renv yaml config.yaml            # print resolved YAML to stdout
renv yaml config.yaml > out.yaml
```

`bw://` and `vault://` values are resolved in place; all other values are passed through unchanged.

### Other commands

| Command | Description |
|---------|-------------|
| `renv shell-init [--shell bash\|zsh\|fish]` | Emit shell functions and start watcher (silent) |
| `renv status` | Show cache status and currently loaded variables |
| `renv clear-cache` | Remove cache, session, and local-password files |
| `renv watch` | Background daemon for sleep/lock events (started by shell-init) |
| `renv --version` | Print version |

---

## Secret reference URI formats

| Format | Fetches |
|--------|---------|
| `bw://folder/item` | Bitwarden — password field (default) |
| `bw://folder/item/username` | Bitwarden — username field |
| `bw://folder/item/note` | Bitwarden — notes field |
| `bw://folder/item/totp` | Bitwarden — TOTP code |
| `bw://folder/item/field:name` | Bitwarden — custom field named `name` |
| `bw://collection:name/item` | Bitwarden — item in a collection |
| `vault://secret/path#field` | HashiCorp Vault KV v2 — `field` key at path |

---

## Configuration

`renv` loads settings from a YAML config file, environment variables, and CLI flags — in that order (CLI flags win).

**Config file location:** `~/.config/renv/config.yaml` (respects `$XDG_CONFIG_HOME`)  
**Override with:** `--config /path/to/config.yaml`

```yaml
log:
  level: warn     # debug | info | warn | error  (env: RENV_LOG_LEVEL)
  format: text    # text | json                  (env: RENV_LOG_FORMAT)

cache:
  max_age: 8h               # Bitwarden item cache TTL  (env: RENV_CACHE_MAX_AGE)
  isolated: false           # per-terminal auth mode    (env: RENV_ISOLATED)
  password_grace_period: 0  # re-prompt window          (env: RENV_PASSWORD_GRACE_PERIOD)

timeouts:
  bitwarden: 30s   # bw subprocess timeout   (env: RENV_TIMEOUT_BITWARDEN)
  vault: 30s       # vault subprocess timeout (env: RENV_TIMEOUT_VAULT)

ui:
  border: true     # rounded panel on TTY; env: RENV_UI_BORDER
```

### Environment variables

| Variable | Description |
|----------|-------------|
| `RENV_LOG_LEVEL` | `debug` / `info` / `warn` / `error` |
| `RENV_LOG_FORMAT` | `text` (default) or `json` |
| `RENV_CACHE_MAX_AGE` | Cache TTL (Go duration, e.g. `4h`) |
| `RENV_ISOLATED` | `true` — require local password per terminal |
| `RENV_PASSWORD_GRACE_PERIOD` | Re-prompt window within same terminal (e.g. `5m`) |
| `RENV_TIMEOUT_BITWARDEN` | Timeout for `bw` subprocess calls (e.g. `60s`) |
| `RENV_TIMEOUT_VAULT` | Timeout for `vault` subprocess calls (e.g. `60s`) |
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive / CI) |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (non-interactive) |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) |
| `VAULT_ADDR` | Vault server URL |
| `VAULT_TOKEN` | Vault authentication token |

---

## Security model

`renv` uses two distinct passwords that must never be confused:

| Password | Purpose | How it is used |
|----------|---------|----------------|
| **BWPassword** | Bitwarden master password | Passed to `bw unlock` via stdin — never as a CLI argument, never persisted |
| **LocalPassword** | Local cache encryption key | AES-256-CBC encrypts the `/dev/shm` cache — held in process memory only, never written to disk |

The encrypted cache uses PBKDF2-SHA256 key derivation (100,000 iterations). Default TTL is 8 hours. Pass `--no-cache` to disable caching entirely.

---

## Special integrations

### direnv

Add a `use_renv` helper to `~/.config/direnv/direnvrc`:

```bash
use_renv() {
  local file="${1:-.env}"
  watch_file "$file"
  eval "$(renv unload 2>/dev/null || true)"
  eval "$(renv resolve "$file")"
}
```

Then in your project's `.envrc`:

```bash
use renv .env
```

Secrets load when you enter the directory and unload when you leave. `watch_file` triggers a re-run when the `.env` changes. First entry prompts for your Bitwarden password; subsequent entries within the cache TTL skip the prompt.

When `renv` detects a direnv context (`DIRENV_DIR` or `DIRENV_FILE` is set) it automatically switches to compact non-TTY output and omits the `EXIT` trap (direnv manages the lifecycle itself).

To enable debug logging inside direnv, set `RENV_LOG_LEVEL` in `.envrc` before the `use renv` call:

```bash
export RENV_LOG_LEVEL=debug
use renv .env
```

### Nix shell

When `IN_NIX_SHELL` is set, `renv` omits the `EXIT` trap from stdout — Nix manages the shell lifecycle itself.

### Sleep and screen-lock (Linux)

`renv watch` listens for D-Bus signals from **systemd-logind**:

| Event | Action |
|-------|--------|
| Screen locked (`loginctl lock-session`) | Env vars unloaded from all open shells; cache kept |
| System suspend / hibernate | Cache, sessions, and local passwords cleared; full re-auth required on wake |

`renv watch` is started automatically by `renv shell-init`.

> **Note:** Locking the screen directly (e.g. `swaylock`, `waylock`) without going through `loginctl` does **not** emit the D-Bus signal. `renv` will not receive the event.

For automatic locking with **swayidle**:

```bash
exec swayidle -w \
    timeout 300 'loginctl lock-session' \
    before-sleep 'loginctl lock-session'
```
