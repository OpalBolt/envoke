# Secure Handling of Secrets

Two small Go tools for working with secrets without ever storing them in plaintext.

- **`renv`** (**r**emote **env**) — replace secret values in `.env` and YAML files with `bw://` and `vault://` references; secrets are fetched at runtime and injected into your shell or process
- **`kctx`** (**k**eyless **c**on**t**e**x**t) — switch Kubernetes contexts by fetching kubeconfig files ephemerally from Vault or Bitwarden; no credentials linger on disk

> **Approved backends:** [HashiCorp Vault](https://www.vaultproject.io/) for team/project secrets · [Bitwarden](https://bitwarden.com/) for personal credentials

## How it works

Instead of putting actual secrets into your config files, you use reference URIs:

```bash
# .env — safe to commit
DB_PASS=bw://prod/database/password
API_KEY=vault://secret/myapp#api_key
```

`renv` resolves these at runtime, fetches the values from the secret store, and either injects them into your shell or passes them directly to a subprocess. Nothing is written to disk — secrets live only in process memory or a short-lived encrypted cache in `/dev/shm`.

`kctx` does the same for Kubernetes: it fetches a kubeconfig from Vault or Bitwarden, writes it to a tmpfile in `/dev/shm`, and exports `KUBECONFIG` pointing at it. The file is deleted when your shell exits.

Both tools render **colored, styled output** (via [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)) — a rounded-border panel to stderr showing exactly what was loaded and where it came from. When running in a pipe or under direnv, the output switches automatically to a compact plain-text format.

## Installation

### Nix flake

```bash
# Ad-hoc (no config changes)
nix profile install github:eficode/envoke
nix profile install github:eficode/envoke#envoke
```

Or as a flake input in your NixOS / home-manager config:

```nix
inputs.envoke.url = "github:eficode/envoke";
# Then add to environment.systemPackages / home.packages:
#   inputs.envoke.packages.${system}.envoke
```

### Go

```bash
go install github.com/eficode/secure-handling-of-secrets/cmd/envoke@latest
```

## renv — remote env

### Shell setup

Add **once** to your shell config — then `renv resolve` and `renv unload` modify the current shell with no `eval` boilerplate:

```bash
# ~/.bashrc or ~/.zshrc
eval "$(renv shell-init)"

# fish: ~/.config/fish/config.fish
renv shell-init --shell fish | source
```

`renv shell-init` is completely **silent** when sourced — it only defines shell functions and starts the background watcher; it prints nothing to the terminal.

After setup:

```bash
renv resolve .env   # fetch secrets and load them into your shell
renv unload         # unset them when done
```

`renv resolve` prints a styled panel to **stderr** showing each variable and its source:

```
╭─ renv: loaded .env ────────-─────────────────╮
│  DB_PASS   bw://prod/database/password       │
│  API_KEY   vault://secret/myapp#api_key      │
╰──────────────────────────────────────────────╯
```

### Run a single command with secrets injected

No shell changes required — secrets are passed directly to the subprocess:

```bash
renv exec -- docker compose up
renv exec -- pytest --verbose
renv exec --env staging.env -- ./deploy.sh
```

### direnv integration

Add to `~/.config/direnv/direnvrc`:

```bash
use_envoke() {
  local file="${1:-.env}"
  watch_file "$file"
  eval "$(envoke unload 2>/dev/null || true)"
  eval "$(envoke resolve "$file")"
}
```

Then in your project's `.envrc`:

```bash
use envoke .env
```

Secrets are loaded when you enter the directory and unloaded when you leave. When `envoke` detects a direnv context (`DIRENV_DIR` or `DIRENV_FILE` is set) it automatically switches to compact non-TTY output and skips the EXIT trap (direnv manages that lifecycle itself).

### YAML files

```bash
renv yaml config.yaml          # print resolved YAML to stdout
renv yaml config.yaml > out.yaml
```

### Secret reference URI formats

```
bw://folder/item                  # Bitwarden → password field (default)
bw://folder/item/username         # Bitwarden → username field
bw://folder/item/note             # Bitwarden → notes field
bw://folder/item/totp             # Bitwarden → TOTP code
bw://folder/item/field:api_key    # Bitwarden → custom field
bw://collection:name/item         # Bitwarden collection
vault://secret/path#field         # HashiCorp Vault KV v2
```

See [docs/renv.md](docs/renv.md) for the full reference including configuration, caching behaviour, and the two-password security model.

## kctx — keyless context

### Shell setup

```bash
# ~/.bashrc or ~/.zshrc
eval "$(kctx shell-init)"
```

`kctx shell-init` is completely **silent** when sourced — it only defines the shell wrapper function and starts the background watcher.

### Usage

```bash
kctx switch prod                         # fetch from vault://secret/kubeconfig/prod
kctx switch prod secret/my-kubeconfig    # custom Vault path
kctx switch prod bw://kube/prod-config   # fetch from Bitwarden

kctx unload                              # unset KUBECONFIG and delete tmpfile
kctx status                              # show current KUBECONFIG path and context
kctx clear-cache                         # remove all kctx Bitwarden cache files
```

Each `kctx switch` writes a fresh tmpfile to `/dev/shm` (falling back to `/tmp`) and registers `trap 'kctx unload' EXIT` so the file is cleaned up automatically when your shell exits. A styled panel is printed to stderr:

```
╭─ kctx: prod ──────────────────────────────────────╮
│  KUBECONFIG   /dev/shm/kctx-a3f2b1                │
│  Context      prod-cluster                        │
│  Source       vault://secret/kubeconfig/prod      │
╰───────────────────────────────────────────────────╯
```

See [docs/kctx.md](docs/kctx.md) for the full reference.

## Security model

| Property | Detail |
|----------|--------|
| No plaintext on disk | Secrets live in process memory or an AES-256 encrypted cache in `/dev/shm` |
| Folder-scoped Bitwarden access | Only items in referenced folders are fetched (not your entire vault) |
| Batch pre-fetch | All Bitwarden folders in a `.env` file are fetched in a single unlock — one password prompt per session |
| Passwords via stdin | Master passwords are passed to `bw` via stdin, never as CLI arguments |
| RAM-backed tmpfiles | `/dev/shm` (Linux tmpfs, cleared on reboot); `/tmp` fallback on macOS |
| Encrypted cache | AES-256-CBC with PBKDF2-SHA256 key derivation; cache TTL defaults to 8 hours |
| Exit cleanup | Shell functions register `EXIT` traps to unset env vars and delete tmpfiles |
| Sleep/lock integration | On Linux, `renv watch` and `kctx watch` listen for D-Bus signals from systemd-logind. Lock → unloads secrets (cache kept). Sleep → also clears cache and sessions. |

⚠️ **Known limitations:** root can read `/dev/shm`; kernel swap may write decrypted data to disk; macOS has no `/dev/shm` equivalent.

## Development

```bash
nix develop          # enter dev shell (Go, goreleaser, bw, vault CLIs)
make build           # build renv and kctx to bin/
make test            # run all tests
make test-race       # run tests with -race (used in CI)
make lint            # go vet
make fmt             # gofmt -w .
```

### Updating Go dependencies

After `go mod tidy` or any `go.mod` change, update `vendorHash` in `flake.nix`:

1. Set `vendorHash = pkgs.lib.fakeHash;` in the `common` block
2. Run `nix build` — it fails with the correct hash:
   ```
   got: sha256-<correct hash>
   ```
3. Replace `pkgs.lib.fakeHash` with that value

## Docs

- [renv reference](docs/renv.md)
- [kctx reference](docs/kctx.md)
