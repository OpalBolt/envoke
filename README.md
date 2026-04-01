# Secure Handling of Secrets

Go CLI tooling for safely fetching secrets from Bitwarden and HashiCorp Vault into shell environments and Kubernetes contexts.

> **Approved backends:** [HashiCorp Vault](https://www.vaultproject.io/) for team/project secrets · [Bitwarden](https://bitwarden.com/) for personal credentials

## Tools

| Binary | Purpose |
|--------|---------|
| [`renv`](docs/renv.md) | Resolve `bw://` and `vault://` references in `.env` and YAML files |
| [`kctx`](docs/kctx.md) | Fetch ephemeral kubeconfig files from Vault or Bitwarden |

## Installation

### Nix Flake

Add the repo as a flake input and include the packages in your configuration:

```nix
# flake.nix
{
  inputs = {
    secure-handling-of-secrets.url = "github:eficode/secure-handling-of-secrets";
  };

  outputs = { self, nixpkgs, secure-handling-of-secrets, ... }: {
    # NixOS or home-manager — add to environment.systemPackages / home.packages:
    #   secure-handling-of-secrets.packages.${system}.renv
    #   secure-handling-of-secrets.packages.${system}.kctx
    #   secure-handling-of-secrets.packages.${system}.default  # both binaries
  };
}
```

Or install ad-hoc without touching your config:

```bash
nix profile install github:eficode/secure-handling-of-secrets        # both (default)
nix profile install github:eficode/secure-handling-of-secrets#renv
nix profile install github:eficode/secure-handling-of-secrets#kctx
```

### Go

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/renv@latest
go install github.com/eficode/secure-handling-of-secrets/cmd/kctx@latest
```

## Quick start

```bash
# Use secret references in .env
echo 'DB_PASS=bw://prod/database/password' > .env

# Resolve and load into your shell
eval "$(renv resolve .env)"

# Unload when done
eval "$(renv unload)"

# With direnv — add use_renv to ~/.config/direnv/direnvrc:
cat >> ~/.config/direnv/direnvrc << 'EOF'
use_renv() {
  local file="${1:-.env}"
  watch_file "$file"
  eval "$(renv unload 2>/dev/null || true)"
  eval "$(renv resolve "$file")"
}
EOF

# Then in your project's .envrc:
echo 'use renv .env' >> .envrc
direnv allow
```

See [docs/renv.md](docs/renv.md) for full details.

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

### Updating dependencies

After running `go mod tidy` or changing any dependency in `go.mod`, the
`vendorHash` in `flake.nix` must be updated:

1. Set `vendorHash = pkgs.lib.fakeHash;` in the `common` block of `flake.nix`
2. Run `nix build` — it will fail with a hash mismatch showing the correct value:
   ```
   specified: sha256-AAAA...
      got:    sha256-<correct hash>
   ```
3. Replace `pkgs.lib.fakeHash` with the `got:` hash string

## Docs

- [renv reference](docs/renv.md)
- [kctx reference](docs/kctx.md)
- [Migration guide from Bash/Python](docs/migration.md)
- [Rewrite rationale](rewrite.md)
