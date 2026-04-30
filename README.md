# envoke

**envoke** *(env + invoke)* loads secrets from Bitwarden into your shell and manages named kubeconfigs — all from a single `.env` file.

Add shell integration once (see [Installation](docs/install.md#shell-integration)):

```bash
eval "$(envoke shell-init)"   # add to ~/.bashrc or ~/.zshrc
```

Then in any directory with a `.env` file:

```bash
# .env
DB_PASSWORD=bw://database/prod-db
KCTX_PROD=bw://kubernetes/prod-cluster

# Load secrets + kubeconfigs
envoke resolve .env

# Switch kubeconfig
envoke switch prod

# Unload everything
envoke unload
```

Secrets are cached in `/run/user/<uid>` (or `/dev/shm` as fallback) and cleared automatically on shell exit, screen lock, or system sleep.

## Documentation

- [Installation & setup](docs/install.md)
- [Configuration](docs/config.md)
- [Command reference](docs/reference.md)
- [Known limitations](docs/limitations.md)
- [direnv integration](docs/direnv.md)
- [Nix integration](docs/nix.md)

## License

[MIT](LICENSE) © OpalBolt
