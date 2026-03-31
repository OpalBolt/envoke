# Migrating from Bash/Python Scripts

## From `resolve-env-refs.sh`

| Old | New |
|-----|-----|
| `source resolve-env-refs.sh .env` | `source <(renv resolve --file .env)` |
| `resolve_yaml_refs.py config.yaml` | `renv yaml config.yaml` |
| `BW_SESSION=xxx resolve-env-refs.sh .env` | `BW_SESSION=xxx renv resolve --file .env` |

### URI format is unchanged

```bash
DB_PASSWORD=bw://my-folder/my-item           # still works
DB_PASSWORD=bw://my-folder/my-item/field:key # still works
API_KEY=vault://secret/myapp#api_key         # still works
```

## From `kctx.sh`

| Old | New |
|-----|-----|
| `source kctx.sh prod` | `source <(kctx-bin switch prod)` or `kctx prod` with shell integration |

## Key differences

- Encrypted cache is now AES-256-CBC with PBKDF2 key derivation (same algorithm as before)
- Cache is stored in `/dev/shm` on Linux (same as before), `/tmp` on macOS
- Sleep/lock cleanup hooks are now available on Linux via D-Bus
