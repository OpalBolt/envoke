# renv — Secret Reference Resolver for .env Files

`renv` resolves `bw://` and `vault://` secret references in `.env` and YAML files.

## Installation

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/renv@latest
```

## Usage

### Zero-eval setup (recommended for interactive use)

Add this **once** to your shell config — then `renv resolve .env` just works, no eval needed:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(renv init)"

# fish: ~/.config/fish/config.fish
renv init --shell fish | source
```

After that, simply:

```bash
renv resolve .env      # loads variables into your shell
renv unload            # unloads them when done
```

### Run a command with secrets injected (no eval, no shell function)

```bash
renv exec -- myprogram --flag value
renv exec --env secrets.env -- myprogram
```

`renv exec` resolves the `.env` file and runs the given command with those variables
set in its environment. The current shell is **not** modified. This is ideal for
scripts, CI pipelines, and one-off commands.

### Manual eval (original behaviour)

If you prefer not to use `renv init`, you can always eval explicitly:

```bash
eval "$(renv resolve .env)"
eval "$(renv unload)"
```

### direnv integration

The recommended way to integrate renv with direnv is via a `use_renv` helper defined
in `~/.config/direnv/direnvrc`. This lets direnv fully own the load/unload lifecycle:
variables are loaded when you enter the directory and **automatically unloaded** when
you leave.

**~/.config/direnv/direnvrc**

```bash
use_renv() {
  local file="${1:-.env}"
  watch_file "$file"
  # Unset any variables from a previous renv load so direnv can track them cleanly.
  eval "$(renv unload 2>/dev/null || true)"
  eval "$(renv resolve "$file")"
}
```

**Your project's .envrc**

```bash
use renv .env
```

`watch_file "$file"` tells direnv to re-run `.envrc` whenever your `.env` changes.

The first run prompts for your Bitwarden master password; subsequent re-entries
(within 8 hours) reuse the stored session and encrypted cache — no re-prompt.

> **Note:** Variables are unloaded by direnv when you leave the directory, so
> `renv unload` is not needed in the normal direnv workflow.  Use it only when
> loading secrets manually (without direnv) via `eval "$(renv resolve .env)"`.

#### Logging with direnv

Because `renv` runs as a subprocess of direnv, `--log-level` and `--verbose` flags
cannot be passed directly. Enable debug output by setting `RENV_LOG_LEVEL` in your
`.envrc` **before** the `use renv` call — direnv exports it into the subprocess
environment:

```bash
# .envrc
export RENV_LOG_LEVEL=debug   # remove or set to 'warn' when done
use renv .env
```

All log output goes to stderr, so it appears in the terminal but never pollutes the
exported variables.

### Manual load / unload (without direnv)

When not using direnv, load secrets into your current shell with `eval`:

```bash
eval "$(renv resolve .env)"
```

To unload (unset all variables that were exported):

```bash
eval "$(renv unload)"
```

> **Note:** Both `renv resolve` and `renv unload` only *print* shell commands —
> you must wrap them in `eval "$(…)"` for the variables to actually be set or unset
> in your shell.

### .env file format

```bash
DB_HOST=localhost
DB_PASSWORD=bw://my-project/database/password
API_KEY=vault://secret/myapp#api_key
```

### Commands

| Command | Description |
|---------|-------------|
| `renv init [--shell bash\|zsh\|fish]` | Print shell function so resolve/unload work without eval |
| `renv resolve [file]` | Resolve and emit exports (default file: `.env`) |
| `renv exec [--env file] -- cmd [args]` | Run command with resolved vars injected (no eval) |
| `renv unload` | Emit unset commands for all tracked variables |
| `renv yaml config.yaml` | Resolve YAML file |
| `renv yaml config.yaml --key database.password` | Extract single value |
| `renv clear-cache` | Remove cache files and stored BW session (preserves var tracking) |
| `renv status` | Show cache status |
| `renv version` | Print version |

## URI formats

| Scheme | Format | Example |
|--------|--------|---------|
| Bitwarden folder | `bw://folder/item[/field]` | `bw://prod/database/password` |
| Bitwarden collection | `bw://collection:name/item[/field]` | `bw://collection:prod/database` |
| Vault KV v2 | `vault://path#field` | `vault://secret/myapp#api_key` |

## Configuration

`renv` loads settings from a YAML config file, environment variables, and CLI flags,
in that order (CLI flags win).

**Default config location:** `~/.config/renv/config.yaml`
(respects `$XDG_CONFIG_HOME`; override with `--config /path/to/config.yaml`)

Copy [`docs/config.yaml`](config.yaml) from this repository as a starting point — it
contains every option with explanations and defaults.

### Logging

```yaml
log:
  level: warn    # debug | info | warn | error  (env: RENV_LOG_LEVEL)
  format: text   # text | json                  (env: RENV_LOG_FORMAT)
```

- `warn` (default) — only warnings and errors
- `info` — adds resolve counts and provider calls
- `debug` — full detail; useful when troubleshooting a failing `bw://` or `vault://` reference

Quick one-shot debug without editing the config:

```bash
renv resolve .env --log-level debug
# or
RENV_LOG_LEVEL=debug renv resolve .env
```

All log output goes to **stderr** and never pollutes the exported variables.

### Cache and session lifetimes

```yaml
cache:
  max_age: 8h               # how long fetched Bitwarden items are cached (env: RENV_CACHE_MAX_AGE)
  session_max_age: 8h       # how long the stored BW session token is kept (env: RENV_SESSION_MAX_AGE)
  isolated: false           # set to true to require local password in each terminal (env: RENV_ISOLATED)
  password_grace_period: 0  # re-prompt window; see below (env: RENV_PASSWORD_GRACE_PERIOD)
```

By default (`isolated: false`, `password_grace_period: 0`) the local cache password is saved to `/dev/shm`
after the first prompt. Subsequent terminals can decrypt the shared encrypted cache without being
prompted — so a second terminal just works if the cache is still warm.

Set `isolated: true` (or `--isolated` / `RENV_ISOLATED=true`) to revert to per-terminal authentication:
every invocation must provide the local password.

#### Password grace period

`password_grace_period` offers a middle ground between always-shared and always-isolated:

```yaml
cache:
  password_grace_period: 1m   # re-prompt after 1 minute of inactivity
```

When set to a non-zero Go duration (e.g. `1m`, `5m`, `30m`):

- The local password store is **keyed per terminal session** (by the parent shell PID). Each new
  terminal must authenticate at least once — Terminal 2 always prompts even if Terminal 1 is
  still within its grace period.
- Within the grace period, the **same** terminal can unload and reload secrets without re-typing
  the password.
- After the grace period the stored key is deleted and the prompt reappears.
- The **encrypted cache files** (the secrets themselves) are still shared across all terminals; only
  the local-password access layer becomes per-terminal.

`renv clear-cache` always removes both the shared password file and all per-terminal session files.

### Timeouts

```yaml
timeouts:
  bitwarden: 30s   # per bw subprocess call (env: RENV_TIMEOUT_BITWARDEN)
  vault: 30s       # per vault subprocess call (env: RENV_TIMEOUT_VAULT)
```

## Environment variables

| Variable | Description |
|----------|-------------|
| `RENV_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `RENV_LOG_FORMAT` | Log format: `text` (default) or `json` |
| `RENV_CACHE_MAX_AGE` | Max age of cached Bitwarden items (Go duration, e.g. `4h`) |
| `RENV_SESSION_MAX_AGE` | Max age of stored Bitwarden session (Go duration, e.g. `24h`) |
| `RENV_ISOLATED` | Set to `true` to require local password in each terminal; disables cross-terminal cache sharing |
| `RENV_PASSWORD_GRACE_PERIOD` | Grace period before re-prompting for local password (Go duration, e.g. `1m`). When set, each terminal authenticates independently. |
| `RENV_TIMEOUT_BITWARDEN` | Timeout for `bw` subprocess calls (Go duration, e.g. `60s`) |
| `RENV_TIMEOUT_VAULT` | Timeout for `vault` subprocess calls (Go duration, e.g. `60s`) |
| `BW_SESSION` | Active Bitwarden session token |
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive) |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (non-interactive) |
| `VAULT_ADDR` | Vault server URL |
| `VAULT_TOKEN` | Vault authentication token |
