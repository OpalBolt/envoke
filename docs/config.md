# Configuration

The config file is optional — sensible built-in defaults work for most setups.

## Config file location

**Default path:** `$XDG_CONFIG_HOME/envoke/config.yaml`  
(typically `~/.config/envoke/config.yaml` when `XDG_CONFIG_HOME` is unset)

**Override at runtime:** `envoke --config /path/to/config.yaml <command>`

## Generate default config

Write a commented default configuration file to the standard location:

```bash
envoke config --init
```

To overwrite an existing config file:

```bash
envoke config --init --force
```

## Config file reference

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

### Log level

Controls verbosity of console output.

| Level | Description |
|-------|-------------|
| `debug` | All messages, including raw API responses and function calls |
| `info` | Information and warnings |
| `warn` | Warnings and errors only (default) |
| `error` | Errors only |

### Log format

Controls output style.

| Format | Description |
|--------|-------------|
| `text` | Human-readable text format (default) |
| `json` | Structured JSON output for log aggregation |

### Cache max_age

Duration string (Go format, e.g. `8h`, `24h`, `30m`) specifying how long Bitwarden folder data is cached before requiring re-authentication.

Default: `8h` (8 hours)

### Timeouts

#### secrets

Timeout for individual `bw` CLI subprocess calls when resolving secrets. If the Bitwarden CLI takes longer than this, envoke abandons the request.

Format: Go duration string (e.g. `30s`, `1m`)  
Default: `30s` (30 seconds)

### UI border

Toggles rounded box borders on summary panels shown by `envoke resolve` and `envoke status`.

Default: `true`

## Environment variable overrides

Environment variables override the config file. CLI flags override both.

```
CLI flags  >  ENVOKE_* env vars  >  config file  >  built-in defaults
```

### Available environment variables

| Variable | Description | Type | Default |
|----------|-------------|------|---------|
| `ENVOKE_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | string | `warn` |
| `ENVOKE_LOG_FORMAT` | Log format: `text` or `json` | string | `text` |
| `ENVOKE_CACHE_MAX_AGE` | Cache TTL (Go duration string, e.g. `8h`, `24h`) | duration | `8h` |
| `ENVOKE_TIMEOUT_SECRETS` | Bitwarden CLI subprocess timeout | duration | `30s` |
| `ENVOKE_UI_BORDER` | Show UI borders: `true` / `false` | boolean | `true` |
| `ENVOKE_BW_PASSWORD` | Bitwarden master password (skips interactive prompt) | string | — |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) | string | — |

## Credential handling

### Interactive prompt (default)

When you run `envoke resolve`, if the Bitwarden cache is cold, you are prompted for your master password:

```
Bitwarden master password: █
```

### Using environment variables

For automation or CI/CD, supply credentials via environment variables:

```bash
export ENVOKE_BW_PASSWORD="your-master-password"
envoke resolve .env
```

Or provide a pre-existing session token to skip `bw unlock` entirely:

```bash
export BW_SESSION="$(bw unlock --raw)"
envoke resolve .env
```

The `BW_SESSION` token avoids storing the master password in the environment.

### Cache lifetime

After successful authentication:
- Bitwarden folder data is cached in `/run/user/<uid>` (or `/dev/shm` / `/tmp` as fallback) for the duration of `ENVOKE_CACHE_MAX_AGE` (default: 8 hours)
- Within the TTL, only your local password is prompted — Bitwarden is not contacted
- After the TTL or after `envoke clear-cache`, both passwords are required again

## Examples

### Minimal config

```yaml
log:
  level: info
```

### Development environment

```yaml
log:
  level: debug
  format: json

cache:
  max_age: 1h

ui:
  border: false
```

### CI/CD environment

```yaml
log:
  level: error
  format: json

cache:
  max_age: 24h

timeouts:
  secrets: 60s
```

Use `ENVOKE_BW_PASSWORD` or `BW_SESSION` env vars to avoid interactive prompts:

```bash
#!/bin/bash
export ENVOKE_BW_PASSWORD="$CI_BITWARDEN_PASSWORD"
export ENVOKE_LOG_LEVEL="error"
envoke resolve .env.production
```

## Troubleshooting

### "config file not found" error

This is not an error — the config file is optional. envoke will use built-in defaults. To create one:

```bash
envoke config --init
```

### Changing just one setting

Use environment variables instead of editing the config file:

```bash
ENVOKE_LOG_LEVEL=debug envoke resolve .env
```

This is temporary and does not modify the config file.

### Cache not working

envoke stores the cache in the first writable directory from this list: `/run/user/<uid>`, `/dev/shm`, `/tmp`. Check which one is in use:

```bash
ls /run/user/$(id -u)/
```

If only `/tmp` is available, secrets are disk-backed (not RAM-backed) but are still removed on exit.

### Slow Bitwarden requests

If `bw` CLI calls are timing out:

1. Increase `ENVOKE_TIMEOUT_SECRETS`:
   ```bash
   ENVOKE_TIMEOUT_SECRETS=60s envoke resolve .env
   ```

2. Or update the config file:
   ```yaml
   timeouts:
     secrets: 60s
   ```

3. Or check your Bitwarden vault for large items or network connectivity issues.
