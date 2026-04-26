# envoke

**envoke** *(env + invoke)* resolves secrets from Bitwarden into your shell environment and manages secure named contexts using an encrypted on-disk cache in `/dev/shm` (RAM-backed when available), with `/tmp` fallback.

```bash
# .env
DB_PASSWORD=bw://database/prod-db
KCTX_PROD=bw://kubernetes/prod-cluster
```

```bash
eval "$(envoke resolve .env)"   # load secrets + kubeconfigs
eval "$(envoke kctx prod)"      # switch to prod kubeconfig
envoke unload                   # clear everything
```

## What it does

| Subcommand | Name | Purpose |
|------------|------|---------|
| `envoke renv` | *remote env* | Resolves `bw://` refs from `.env` files into shell exports |
| `envoke kctx` | *Keyless ConTeXt* | Loads and switches named contexts (kubeconfigs, etc.) from Bitwarden |
| `envoke resolve` | | Runs both in a single pass |

Secrets and kubeconfigs are cached in `/dev/shm` (RAM-backed tmpfs), encrypted with AES-256. The cache is cleared on shell exit or sleep. On system lock/unlock, env vars are unloaded and managed kubeconfig tempfiles are removed.

## Documentation

- [Installation](docs/installation.md)
- [Introduction](docs/introduction.md)
- [Setup](docs/setup.md)
- [Usage](docs/usage.md)
- [direnv integration](docs/direnv.md)
- [Nix integration](docs/nix.md)

## License

[MIT](LICENSE) © OpalBolt
