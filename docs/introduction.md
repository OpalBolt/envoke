# Introduction

## What is envoke?

**envoke** *(env + invoke)* is a CLI tool for securely loading secrets into your shell. It reads a `.env` file, resolves any secret references against Bitwarden or HashiCorp Vault, and emits `export` statements you evaluate in your shell. It also manages named encrypted contexts (such as kubeconfigs) the same way.

The tool ships as a single binary (`envoke`) with two logical subcommands:

- **renv** *(remote env)* — resolves environment secrets from `.env` files
- **kctx** *(Keyless ConTeXt)* — manages named contexts fetched from Bitwarden or Vault

You can use `envoke resolve` to handle both in one pass, or call `envoke renv` / `envoke kctx` for each individually.

## Why envoke?

Secrets should never sit in plaintext files, shell history, or process arguments. envoke solves this by:

- Storing fetched secrets **only in encrypted RAM** (`/dev/shm`)
- Prompting for passwords **interactively** — never reading them from args or plaintext files
- Passing Bitwarden passwords via **stdin**, not CLI arguments
- **Clearing the cache** automatically on shell exit, screen lock, or system sleep
- **Tracking loaded variables** so they can be unset cleanly with `envoke unload`

## Core concepts

### Secret references

A `.env` file can contain literal values or secret references:

```bash
PLAIN=literal
DB_PASSWORD=bw://database/prod-db
API_TOKEN=vault://secret/api#token
```

References are resolved at load time. Only the resolved value reaches your shell.

### Bitwarden URI format

```
bw://folder/item                    # password field (default)
bw://folder/item/username           # username field
bw://folder/item/note               # notes field
bw://folder/item/totp               # TOTP code
bw://folder/item/field:custom_name  # custom field
bw://collection:name/item           # item in a collection
```

### Vault URI format

```
vault://path#field                  # KV v2 — field fragment is required
```

### Kubeconfig directives

Lines prefixed with `KCTX_` in a `.env` file are treated as named context sources, not environment variables:

```bash
KCTX_PROD=bw://kubernetes/prod-cluster
KCTX_STAGING=vault://secret/kubeconfig/staging#kubeconfig
```

`envoke resolve` will load each item into a named local store instead of exporting it as a variable. You then switch between them with `kctx prod` / `kctx staging`.

> **Note:** kctx currently focuses on kubeconfigs but is designed to manage other types of named contexts in the future.

### Encrypted cache

After fetching secrets from Bitwarden, the result is cached in `/dev/shm` (RAM-backed, never written to disk) using AES-256-CBC with a key derived from your local password. This means:

- Subsequent resolves within the TTL (default: 8 hours) only prompt for your local password
- Bitwarden is not contacted again until the cache expires or is cleared

### Two-password model

| Password | Purpose | Where it goes |
|----------|---------|---------------|
| **Bitwarden password** | Unlock your Bitwarden vault | Piped to `bw unlock` via stdin; never persisted |
| **Local password** | Encrypt/decrypt the `/dev/shm` cache | Held in process memory only |

In automation, set `RENV_BW_PASSWORD` and `RENV_LOCAL_PASSWORD` environment variables to skip interactive prompts.

## Architecture overview

```
envoke resolve .env
       │
       ├── Parse .env
       │       ├── KCTX_* entries  ─→  kctx: fetch + encrypt to /dev/shm/kctx-kc-{uid}-{name}.enc
       │       └── env entries     ─→  renv: fetch + encrypt to /dev/shm/renv-{uid}-{hash}.enc
       │
       └── Emit:  export KEY='value'
                  (+ trap / unload token)
```

All sensitive files in `/dev/shm` have permissions `0600` and fall back to `/tmp` if `/dev/shm` is unavailable.
