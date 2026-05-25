# Reference

## .env file syntax

```bash
# Comments start with #
KEY=value
KEY="value with spaces"
KEY='value with spaces'
export KEY=value        # export prefix is accepted and stripped
```

### Secret references

```bash
KEY=bw://folder/item                    # secure note body or password field
KEY=bw://folder/item/username           # username field
KEY=bw://folder/item/note               # notes / secure note (explicit)
KEY=bw://folder/item/totp               # TOTP code
KEY=bw://folder/item/field:custom_name  # custom field by name
KEY=bw://collection:name/item           # item in a named collection
```

For login items, the default field is `password`. For secure note items with no login, the note body is returned automatically — no `/note` suffix needed.

### CTX_ context groups

`CTX_<GROUP>=<uri>#<ENVVAR>` entries write a secret to a tmpfile in `/dev/shm` and export `$ENVVAR` pointing at it. The fragment determines the target env var — envoke places no meaning on it beyond that:

```bash
CTX_PROD=bw://k8s/prod#KUBECONFIG
CTX_PROD=bw://talos/prod#TALOSCONFIG
CTX_PROD=bw://aws/prod#AWS_SHARED_CREDENTIALS_FILE
CTX_META=bw://tokens/github/password#GITHUB_TOKEN
CTX_META=bw://docker/config#DOCKER_CONFIG
```

- The same group name can appear on multiple lines — all entries belong to that group
- The same URI can appear in multiple groups — it is fetched once per resolve (BW folder cache)
- Group names are lowercased: `CTX_PROD` → group `prod`

**Reserved group name: `meta`**
`CTX_META` entries are a persistent baseline loaded on every `envoke switch`. They cannot be switched away from. If a group entry collides with a META entry, the group wins with a warning.

**`ENVOKE_DEFAULT_GROUP`**
Consumed by envoke during resolve, not exported as an env var. Auto-switches to the named group after all secrets are fetched:

```bash
ENVOKE_DEFAULT_GROUP=prod
```

### Built-in content validators

When the `#ENVVAR` fragment matches a known name, envoke validates the secret content before writing it to a tmpfile:

| Fragment | Validation |
|---|---|
| `#KUBECONFIG` | must contain `apiVersion` |
| `#TALOSCONFIG` | must contain `context:` |
| `#AWS_SHARED_CREDENTIALS_FILE` | must contain `[default]` or `[profile ` |
| `#DOCKER_CONFIG` | must be valid JSON |
| anything else | no validation — written as-is |

### Migration from KCTX_

The old `KCTX_<name>=bw://...` syntax is no longer supported. If an entry with a `KCTX_` prefix is encountered, envoke prints a migration error and exits. Migrate to `CTX_`:

```bash
# Before
KCTX_PROD=bw://kubernetes/prod-cluster

# After
CTX_PROD=bw://kubernetes/prod#KUBECONFIG
```

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
| Shell exit | Unload secrets, remove context tmpfiles, clear cache | ✓ | ✓ |
| Screen lock | Unset loaded variables in open shells | ✓ | ✗ |
| System sleep | Clear cache — full re-auth required on wake | ✓ | ✓ (after wake) |

On Linux, lock/sleep detection uses D-Bus (systemd-logind). On macOS, sleep detection uses a timer-drift heuristic; screen lock is not detected. See [Known limitations](limitations.md).
