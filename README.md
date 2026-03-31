# Secure Handling of Secrets

Go CLI tooling for safely fetching secrets from Bitwarden and HashiCorp Vault into shell environments and Kubernetes contexts.

> **Approved backends:** [HashiCorp Vault](https://www.vaultproject.io/) for team/project secrets · [Bitwarden](https://bitwarden.com/) for personal credentials

## Tools

| Binary | Purpose |
|--------|---------|
| [`renv`](docs/renv.md) | Resolve `bw://` and `vault://` references in `.env` and YAML files |
| [`kctx`](docs/kctx.md) | Fetch ephemeral kubeconfig files from Vault or Bitwarden |

## Quick start

```bash
# Install both binaries
go install github.com/eficode/secure-handling-of-secrets/cmd/renv@latest
go install github.com/eficode/secure-handling-of-secrets/cmd/kctx@latest

# Shell integration
source <(renv shell-init)
source <(kctx-bin shell-init)

# Use secret references in .env
echo 'DB_PASS=bw://prod/database/password' > .env
resolve_env_file .env   # exports DB_PASS, registers EXIT unload trap
```

## URI formats

```bash
bw://folder/item                 # Bitwarden folder → password field (default)
bw://folder/item/username        # Bitwarden folder → username field
bw://folder/item/field:api_key   # Bitwarden folder → custom field
bw://collection:name/item        # Bitwarden collection
vault://secret/path#field        # HashiCorp Vault KV v2
```

## Development

```bash
nix develop          # enter dev shell (Go, goreleaser, bw, vault)
make build           # build renv and kctx to bin/
make test            # run all tests
make lint            # go vet
```

## Docs

- [renv reference](docs/renv.md)
- [kctx reference](docs/kctx.md)
- [Migration guide from Bash/Python](docs/migration.md)
- [Rewrite rationale](rewrite.md)
