# envoke

Fetch secrets from Bitwarden or HashiCorp Vault at runtime and inject them into your shell — nothing stored in plaintext, nothing left on disk.

- **`renv`** — resolve `bw://` / `vault://` references in `.env` and YAML files; load secrets into your shell or pass them directly to a subprocess
- **`kctx`** — switch Kubernetes contexts by fetching kubeconfigs ephemerally from Vault or Bitwarden; `KUBECONFIG` is set only in your current shell and cleaned up on exit

```bash
# .env — safe to commit
DB_PASS=bw://prod/database
API_KEY=vault://secret/myapp#api_key

# load into shell
renv resolve .env

# or run a one-off command
renv exec -- docker compose up

# switch k8s context
kctx switch prod
```

Secrets live in process memory or a short-lived AES-256 encrypted cache in `/dev/shm`. Nothing persists past your session.

## Quick start

```bash
# install
go install github.com/eficode/secure-handling-of-secrets/cmd/envoke@latest

# add to ~/.bashrc or ~/.zshrc
eval "$(envoke shell-init)"
```

## Documentation

- [Installation](docs/installation.md)
- [renv — remote env](docs/renv.md)
- [kctx — keyless context](docs/kctx.md)
