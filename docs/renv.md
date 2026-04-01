# renv — Secret Reference Resolver for .env Files

`renv` resolves `bw://` and `vault://` secret references in `.env` and YAML files.

## Installation

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/renv@latest
```

## Usage

### Evaluating exports

`renv resolve` prints `export KEY=value` statements to stdout. Your shell must evaluate them to set the variables:

```bash
eval "$(renv resolve .env)"
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
| `renv resolve [file]` | Resolve and emit exports (default file: `.env`) |
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

## Environment variables

| Variable | Description |
|----------|-------------|
| `BW_SESSION` | Active Bitwarden session token |
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive) |
| `VAULT_ADDR` | Vault server URL |
| `VAULT_TOKEN` | Vault authentication token |
