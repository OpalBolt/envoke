# renv — Secret Reference Resolver

`renv` resolves `bw://` and `vault://` secret references in `.env` and YAML files,
injecting secrets into your shell or a subprocess at runtime. Nothing is written to
disk in plaintext — secrets live only in process memory or a short-lived AES-256
encrypted cache in `/dev/shm`.

## Installation

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/renv@latest
```

Or via Nix:

```bash
nix profile install github:eficode/secure-handling-of-secrets#renv
```

## Shell setup (recommended)

Add **once** to your shell config to get `renv resolve` and `renv unload` working
without manual `eval` boilerplate:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(renv shell-init)"

# fish: ~/.config/fish/config.fish
renv shell-init --shell fish | source
```

`renv shell-init` is completely **silent** when sourced — it defines shell wrapper
functions and starts the background watcher (`renv watch`) without printing anything
to the terminal.

After setup:

```bash
renv resolve .env   # fetch secrets, load into current shell
renv unload         # unset all tracked variables
```

### Manual eval (without shell-init)

If you prefer not to use `renv shell-init`:

```bash
eval "$(renv resolve .env)"
eval "$(renv unload)"
```

## Commands

| Command | Description |
|---------|-------------|
| `renv shell-init [--shell bash\|zsh\|fish]` | Emit shell functions + start watcher (silent) |
| `renv resolve [file]` | Resolve `.env` file; emit exports to stdout, panel to stderr |
| `renv exec [--env file] -- cmd [args]` | Run command with resolved vars injected (no eval) |
| `renv unload` | Emit unset commands for all tracked variables |
| `renv yaml file` | Resolve YAML file and print to stdout |
| `renv status` | Show cache status and currently loaded variables |
| `renv clear-cache` | Remove cache, session, and local-password files |
| `renv watch` | Background daemon for sleep/lock events (started by shell-init) |
| `renv --version` | Print version |

## Output format

`renv resolve` writes two things:

- **stdout** — shell statements for `eval` (or for the shell wrapper):
  ```
  export DB_PASS='s3cr3t'
  export API_KEY='abc123'
  trap 'renv clear-cache' EXIT
  ```
  The `trap` line is omitted when running under direnv (`DIRENV_DIR` / `DIRENV_FILE`
  is set) or inside a Nix shell (`IN_NIX_SHELL` is set), because those environments
  manage the lifecycle themselves.

- **stderr** — a styled panel showing what was loaded and where it came from:

  On a TTY (rounded-border box):
  ```
  ╭─ renv: loaded .env ─────────────────────────╮
  │  DB_PASS   bw://prod/database/password       │
  │  API_KEY   vault://secret/myapp#api_key      │
  │  APP_ENV   (literal)                         │
  ╰──────────────────────────────────────────────╯
  ```

  On non-TTY stderr (direnv, pipes — compact plain-text):
  ```
  renv: loaded .env
    DB_PASS    bw://prod/database/password
    API_KEY    vault://secret/myapp#api_key
    APP_ENV    (literal)
  ```

`renv unload` similarly emits `unset KEY` statements to stdout and a compact panel
to stderr.

## .env file format

```bash
# Plain values are passed through unchanged
APP_ENV=production

# Secret references — fetched at runtime
DB_PASSWORD=bw://my-project/database/password
API_KEY=vault://secret/myapp#api_key
```

## URI formats

| Scheme | Format | Fetches |
|--------|--------|---------|
| Bitwarden (password) | `bw://folder/item` | Password field |
| Bitwarden (field) | `bw://folder/item/username` | Username field |
| Bitwarden (notes) | `bw://folder/item/note` | Notes field |
| Bitwarden (TOTP) | `bw://folder/item/totp` | TOTP code |
| Bitwarden (custom) | `bw://folder/item/field:name` | Custom field named `name` |
| Bitwarden (collection) | `bw://collection:name/item` | Item in a Bitwarden collection |
| Vault KV v2 | `vault://secret/path#field` | `field` key at the given path |

## Run a command with secrets (no eval)

`renv exec` resolves the `.env` file and runs the given command with secrets set
in its environment. The current shell is **not** modified:

```bash
renv exec -- docker compose up
renv exec -- pytest --verbose
renv exec --env staging.env -- ./deploy.sh
```

Ideal for scripts, CI pipelines, and one-off commands.

## YAML files

```bash
renv yaml config.yaml            # print resolved YAML to stdout
renv yaml config.yaml > out.yaml
```

YAML values that are `bw://` or `vault://` URIs are resolved in place; all other
values are passed through unchanged.

## direnv integration

The recommended way to integrate `renv` with direnv is a `use_renv` helper in
`~/.config/direnv/direnvrc`:

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

`watch_file "$file"` tells direnv to re-run `.envrc` whenever the `.env` changes.
The first run prompts for your Bitwarden master password; subsequent entries within
the cache TTL reuse the encrypted cache — no re-prompt.

When `renv` detects a direnv context (`DIRENV_DIR` or `DIRENV_FILE` is set) it:
- Switches to compact, non-TTY output on stderr
- Omits the `trap` line from stdout (direnv manages the unload lifecycle)

#### Logging with direnv

Because `renv` runs as a subprocess of direnv, pass `RENV_LOG_LEVEL` in `.envrc`:

```bash
# .envrc
export RENV_LOG_LEVEL=debug   # remove or set to warn when done
use renv .env
```

All log output goes to stderr and never pollutes the exported variables.

## Configuration

`renv` loads settings from a YAML config file, environment variables, and CLI flags
(CLI flags win).

**Default config location:** `~/.config/renv/config.yaml`
(respects `$XDG_CONFIG_HOME`; override with `--config /path/to/config.yaml`)

### Logging

```yaml
log:
  level: warn    # debug | info | warn | error  (env: RENV_LOG_LEVEL)
  format: text   # text | json                  (env: RENV_LOG_FORMAT)
```

One-shot debug:

```bash
renv resolve .env --log-level debug
# or
RENV_LOG_LEVEL=debug renv resolve .env
```

### Cache and session lifetimes

```yaml
cache:
  max_age: 8h               # cached Bitwarden item TTL (env: RENV_CACHE_MAX_AGE)
  session_max_age: 8h       # stored BW session token TTL (env: RENV_SESSION_MAX_AGE)
  isolated: false           # per-terminal auth, no sharing (env: RENV_ISOLATED)
  password_grace_period: 0  # re-prompt window (env: RENV_PASSWORD_GRACE_PERIOD)
```

### Timeouts

```yaml
timeouts:
  bitwarden: 30s   # per bw subprocess call (env: RENV_TIMEOUT_BITWARDEN)
  vault: 30s       # per vault subprocess call (env: RENV_TIMEOUT_VAULT)
```

## Two-password security model

`renv` uses two distinct passwords that must never be confused:

| Password | Purpose | How it is used |
|----------|---------|----------------|
| **BWPassword** | Bitwarden master password | Passed to `bw unlock` **via stdin** — never as a CLI argument, never persisted |
| **LocalPassword** | Local cache encryption key | Used to AES-256-CBC encrypt/decrypt the `/dev/shm` cache. Prompted once per session; optionally shared across terminals (disabled by `--isolated`) |

### Encrypted cache

- **Location:** `/dev/shm` (Linux tmpfs) with `/tmp` fallback
- **Encryption:** AES-256-CBC; key = PBKDF2-SHA256(localPassword, salt, 100,000 iterations, 32 bytes)
- **Default TTL:** 8 hours (`RENV_CACHE_MAX_AGE`)
- `--no-cache` sets `Cache.Disabled = true` — `Put`/`Get` become no-ops

### Cross-terminal sharing vs isolation

By default (`isolated: false`) the local cache password is stored in `/dev/shm` after
the first prompt. Subsequent terminals can decrypt the shared encrypted cache without
being prompted again.

Set `isolated: true` (or `RENV_ISOLATED=true`) to require the local password in
every terminal.

#### Password grace period

`password_grace_period` offers a middle ground:

```yaml
cache:
  password_grace_period: 1m   # re-prompt after 1 minute of inactivity per terminal
```

When set to a non-zero duration:

- The local-password store is keyed per terminal (parent shell PID). Each new terminal
  must authenticate at least once.
- Within the grace period, the same terminal can unload and reload secrets without
  re-typing the password.
- After the grace period the stored key is deleted and the prompt reappears.
- The encrypted cache files (the secrets themselves) are still shared across terminals;
  only the local-password access layer becomes per-terminal.

`renv clear-cache` removes both the shared password file and all per-terminal session
files.

## Sleep and screen-lock integration (Linux)

`renv watch` listens for D-Bus signals from **systemd-logind**:

| Event | D-Bus signal | Action |
|-------|-------------|--------|
| Screen locked | `org.freedesktop.login1.Session.Lock` | Env vars unloaded from open shells; cache kept |
| System suspend/hibernate | `org.freedesktop.login1.Manager.PrepareForSleep` | Cache, session, and local passwords cleared; full re-auth required after wake |

`renv watch` is started automatically by `renv shell-init` as a background daemon.

### Requirement: lock via loginctl / D-Bus

The lock signal only reaches `renv` when the session is locked through
`loginctl lock-session` (or anything else that sends the D-Bus signal):

```bash
loginctl lock-session
```

For automatic locking with **swayidle**:

```bash
exec swayidle -w \
    timeout 300 'loginctl lock-session' \
    before-sleep 'loginctl lock-session'
```

Manual keybind (sway):

```
bindsym $mod+l exec loginctl lock-session
```

> **Note:** Invoking a screen locker directly (e.g. `swaylock`, `waylock`) without
> going through `loginctl` does **not** emit the D-Bus signal. `renv` will not
> receive the lock event.

## Environment variables

| Variable | Description |
|----------|-------------|
| `RENV_LOG_LEVEL` | `debug` / `info` / `warn` / `error` |
| `RENV_LOG_FORMAT` | `text` (default) or `json` |
| `RENV_CACHE_MAX_AGE` | Cache TTL (Go duration, e.g. `4h`) |
| `RENV_SESSION_MAX_AGE` | BW session token TTL (Go duration, e.g. `24h`) |
| `RENV_ISOLATED` | `true` to require local password in each terminal |
| `RENV_PASSWORD_GRACE_PERIOD` | Per-terminal re-prompt window (Go duration, e.g. `1m`) |
| `RENV_TIMEOUT_BITWARDEN` | Timeout for `bw` subprocess calls (Go duration, e.g. `60s`) |
| `RENV_TIMEOUT_VAULT` | Timeout for `vault` subprocess calls (Go duration, e.g. `60s`) |
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive / CI) |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (non-interactive) |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) |
| `VAULT_ADDR` | Vault server URL |
| `VAULT_TOKEN` | Vault authentication token |
