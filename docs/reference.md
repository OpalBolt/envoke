# Reference

## .env file syntax

```bash
# Comments start with #
KEY=value
KEY="value with spaces"
KEY='value with spaces'
export KEY=value        # export prefix is accepted and stripped
```

### Bitwarden reference format

```bash
KEY=bw://folder/item                    # password field (default)
KEY=bw://folder/item/username           # username field
KEY=bw://folder/item/note               # notes / secure note
KEY=bw://folder/item/totp               # TOTP code
KEY=bw://folder/item/field:custom_name  # custom field by name
KEY=bw://collection:name/item           # item in a named collection
```

### Kubeconfig directives

Lines prefixed with `KCTX_` load a kubeconfig into the named store rather than being exported as env vars:

```bash
KCTX_PROD=bw://kubernetes/prod-cluster
KCTX_STAGING=bw://kubernetes/staging-cluster
```

The prefix is stripped and the remainder is lowercased to form the store name: `KCTX_PROD` → `prod`.

---

## Global flags

All commands accept these flags:

| Flag | Description |
|------|-------------|
| `--verbose` | Enable debug logging (shorthand for `--log-level=debug`) |
| `--log-level LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `--config PATH` | Path to config file |

---

## Cache behaviour

Bitwarden folder data is cached in `/run/user/<uid>` (systemd-logind tmpfs, mode 0700), falling back to `/dev/shm`, then `/tmp`. The default TTL is 8 hours, configurable via `ENVOKE_CACHE_MAX_AGE` or the config file.

Within the TTL, no password is prompted and Bitwarden is not contacted. After the TTL or after `envoke clear-cache`, the Bitwarden master password is required again.

See [Configuration](config.md) for all cache-related settings.

---

## Automatic cleanup

When shell-init is active:

| Event | Action | Linux | macOS |
|-------|--------|-------|-------|
| Shell exit | Unload secrets, clear cache, kill watcher | ✓ | ✓ |
| Screen lock | Unset loaded variables in open shells | ✓ | ✗ |
| System sleep | Clear cache — full re-auth required on wake | ✓ | ✓ (after wake) |

On Linux, lock/sleep detection uses D-Bus (systemd-logind). On macOS, sleep detection uses a timer-drift heuristic; screen lock is not detected. See [Known limitations](limitations.md).
