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

Add to your `.envrc`:

```bash
eval "$(renv resolve .env)"
```

direnv will source this on folder entry. The first run prompts for your Bitwarden master password; subsequent re-entries (within 8 hours) reuse the stored session and hit the encrypted cache — no re-prompt.

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
| `renv yaml config.yaml` | Resolve YAML file |
| `renv yaml config.yaml --key database.password` | Extract single value |
| `renv clear-cache` | Remove all cache files and stored BW session |
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
