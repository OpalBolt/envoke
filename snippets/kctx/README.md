# kctx

Ephemeral kubeconfig switching via Vault or Bitwarden — credentials land in RAM only, never on disk.

## Install

```bash
./snippets/kctx/install.sh
```

Copies `kctx.sh` to `~/.config/kctx/kctx.sh`.

```bash
./snippets/kctx/install.sh --check   # is it up to date?
./snippets/kctx/install.sh            # install or upgrade
```

Add to `~/.bashrc` or `~/.zshrc`:

```bash
source ~/.config/kctx/kctx.sh
```

---

## Usage

```bash
kctx prod                          # fetch from Vault: secret/k8s/prod
kctx prod secret/infra/k8s/prod    # explicit Vault KV path
kctx prod bw://k8s-prod            # fetch from Bitwarden item (kubeconfig field)
kctx prod bw://k8s-prod/field:cfg  # Bitwarden custom field named "cfg"

kctx_status                        # show active KUBECONFIG and kubectl context
kctx_clear                         # unset KUBECONFIG and delete tmpfile
kctx_cache_clear                   # flush all cached kubeconfigs from RAM
```

---

## Caching

After the first fetch, the kubeconfig is cached in `/dev/shm` (RAM-backed tmpfs on Linux) for **1 hour** (`_KCTX_CACHE_TTL`). Subsequent calls reuse the cache — no round-trip to Vault or Bitwarden.

The cache file is:
- Named by the first 16 hex chars of `SHA-256(vault-path)` — unique per path
- `chmod 600` — inaccessible to other users
- Stored in `/dev/shm` (RAM); falls back to `/tmp` on macOS

The active working copy (pointed to by `$KUBECONFIG`) is always a separate tmpfile deleted on `kctx_clear` or shell exit.

---

## Prerequisites

| Tool | Required for |
|------|-------------|
| `vault` | Vault paths (`VAULT_ADDR` + `VAULT_TOKEN` must be set) |
| `bw` + `jq` | `bw://` paths (`BW_SESSION` must be set) |
| `kubectl` | Context display (optional) |

**Bitwarden session**: `BW_SESSION` must be set before calling `kctx` with a `bw://` path:

```bash
export BW_SESSION=$(bw unlock --raw)
kctx prod bw://k8s-prod
```

The Bitwarden item should have a custom field named `kubeconfig` containing the kubeconfig YAML. Use `/field:name` to specify a different field name.

---

## Security

| Threat | Protected? |
|--------|-----------|
| Other non-root users reading `/dev/shm` | ✅ `chmod 600` |
| Root | ❌ root can read `/dev/shm` |
| macOS `/tmp` | ⚠️ disk-backed; use a RAM disk for equivalent protection |
| SIGKILL / OOM kill | ❌ EXIT trap does not fire; tmpfile stays in RAM until reboot (harmless — `/dev/shm` clears on reboot) |
