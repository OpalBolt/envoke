# envoke

**envoke** *(env + invoke)* resolves secrets from Bitwarden and HashiCorp Vault into your shell environment and manages secure named contexts — all encrypted in RAM.

```bash
# .env
DB_PASSWORD=bw://database/prod-db
API_TOKEN=vault://secret/api#token
KCTX_PROD=bw://kubernetes/prod-cluster
```

```bash
eval "$(envoke resolve .env)"   # load secrets + kubeconfigs
kctx prod                       # switch to prod kubeconfig
envoke unload                   # clear everything
```

## What it does

| Subcommand | Name | Purpose |
|------------|------|---------|
| `envoke renv` | *remote env* | Resolves `bw://` and `vault://` refs from `.env` files into shell exports |
| `envoke kctx` | *Keyless ConTeXt* | Loads and switches named contexts (kubeconfigs, etc.) from Bitwarden/Vault |
| `envoke resolve` | | Runs both in a single pass |

Secrets and kubeconfigs are cached in `/dev/shm` (RAM-backed tmpfs), encrypted with AES-256. The cache is cleared on shell exit, system lock, or sleep.

## Documentation

- [Installation](docs/installation.md)
- [Introduction](docs/introduction.md)
- [Setup](docs/setup.md)
- [Usage](docs/usage.md)
- [direnv integration](docs/direnv.md)
- [Nix integration](docs/nix.md)
