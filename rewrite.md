# Rewrite Plan: Secure Handling of Secrets → Go CLI

> **Context file for AI agents.** This document is the single source of truth for the full rewrite of this repository's shell-based tooling into Go binaries.

---

## 1. Branch Strategy

| Detail | Value |
|---|---|
| Base branch | `anvil/folder-scoped-bw` |
| New branch | `rewrite` (create from base) |
| Merge target | `main` (when rewrite is complete) |

History is preserved — work on `rewrite` until feature-complete, then merge.

### Commit Discipline

Conversational commits must be made often, even for small changes. Every meaningful edit — a new function, a renamed variable, a config tweak — gets its own commit with a clear, descriptive message. This keeps the history reviewable, makes bisecting easier, and ensures progress is never lost.

---

## 2. GitHub Issues Driving This Rewrite

### Issue #5 — Rewrite `resolve-env-refs` in Go ([link](https://github.com/eficode/secure-handling-of-secrets/issues/5))

Replace `snippets/resolve-env-refs/resolve-env-refs.sh` (Bash, ~600 lines) and `snippets/resolve-env-refs/resolve_yaml_refs.py` (Python) with a single Go binary: **`renv`**.

**Why Go over Bash/Python:**

- `/dev/shm` RAM-backed cache is Linux-only — macOS falls back to disk-backed `/tmp`, defeating security guarantees
- Sleep/lock event hooks (#1) require per-platform APIs (systemd, launchd, WTS) — impossible in Bash
- Bash unavailable on Windows
- Python's GC silently copies secret values in memory — no guaranteed zeroing
- Go: single binary, `unix.Mlock`/`VirtualLock`, manual memory control, `//go:build` tags

**Scope for `renv`:**

- Same `bw://folder/item[/field]` and `vault://secret/path#field` URI syntax
- Cross-platform RAM-backed cache (mlock'd memory or OS-appropriate secure temp storage)
- Sleep/suspend + screen-lock cleanup hooks per platform (resolves #1)
- Collections support alongside folder support (resolves #3)
- Simple install: single binary + optional shell integration snippet for `trap`/`unload_env` pattern
- Backward-compatible with existing `.env` file syntax

### Issue #6 — Rewrite `kctx` in Go ([link](https://github.com/eficode/secure-handling-of-secrets/issues/6))

Replace `snippets/kctx/kctx.sh` (Bash, ~550 lines) with a Go binary: **`kctx`**. Companion to `renv`, sharing the same internal packages.

**Scope for `kctx`:**

- Same URI syntax: `kctx <env>`, `kctx <env> <vault-path>`, `kctx <env> bw://<item>`, `kctx <env> bw://<item>/field:<field>`
- Cross-platform RAM-backed kubeconfig cache, reusing `internal/secrets`
- Sleep/suspend + screen-lock cleanup hooks, reusing shared hook layer from #5
- Shell integration snippet (`kctx_clear`, `kctx_status`, EXIT trap) — binary cannot set `KUBECONFIG` in parent shell directly

**Depends on:** #5 for the `internal/secrets` shared layer.

### Issue #1 — Clean Sensitive Data on Events ([link](https://github.com/eficode/secure-handling-of-secrets/issues/1))

Sensitive data (BW tokens, env vars, cached data) must be cleaned on:

- Shell closed
- Computer reboot / shutdown / sleep mode
- Computer locked

This is addressed by building platform-specific cleanup hooks into the Go binaries.

### Issue #3 — Scoped Bitwarden Export ([link](https://github.com/eficode/secure-handling-of-secrets/issues/3))

- Never perform a complete export, not even to RAM
- Support item scoping: `item-name`, `folder/item-name`, `folder/item-name/attribute`
- Support both folders and collections
- Ignore attachments gracefully

---

## 3. What Exists Today (Current Architecture)

### Core tools being replaced

| Current file | Lines | Language | Replacement |
|---|---|---|---|
| `snippets/resolve-env-refs/resolve-env-refs.sh` | ~600 | Bash | `cmd/renv/` |
| `snippets/resolve-env-refs/resolve_yaml_refs.py` | ~200 | Python | `cmd/renv/` (YAML subcommand or flag) |
| `snippets/kctx/kctx.sh` | ~550 | Bash | `cmd/kctx/` |
| `snippets/kctx/install.sh` | ~30 | Bash | Go install script or `go install` |
| `snippets/resolve-env-refs/install.sh` | ~30 | Bash | Go install script or `go install` |

### Key behaviors to preserve

1. **Two-pass secret resolution** — batch-fetch BW folders (one unlock) → cache → emit exports → clear session
2. **RAM-only storage** — `/dev/shm` on Linux; the Go rewrite must use mlock'd memory cross-platform
3. **Encrypted caching** — AES-256-CBC + PBKDF2, per-user, per-account, per-folder
4. **Fail-closed** — exit non-zero if secret retrieval fails; never pass empty credentials
5. **Session lifecycle** — BW_SESSION ephemeral, cleared after fetch; encrypted cache survives across shells
6. **Cleanup on EXIT** — trap unsets all exported variables
7. **URI schemes** — `bw://folder/item[/field]` and `vault://secret/path#field`

### Files to remove (tracked as a separate issue)

The following paths will be **removed** from the repo on the `rewrite` branch. A new GitHub issue should be created to re-add/rewrite them once the Go binaries are stable:

- `guides/` — all guide content
- `examples/` — language-specific examples
- `agents/` — secret-scanner, secret-injector
- `best-practices.md`

This keeps the `rewrite` branch focused solely on the Go tooling. Documentation and auxiliary scripts come back in a later phase.

---

## 4. Target Architecture (Go Project Layout)

Following [golang-standards/project-layout](https://github.com/golang-standards/project-layout):

```
cmd/
  renv/                 # main.go — resolve-env-refs CLI (issue #5)
  kctx/                 # main.go — kubeconfig switcher CLI (issue #6)
internal/
  secrets/              # Shared secret backend abstraction
    bitwarden.go        # bw:// fetch — Bitwarden SDK (github.com/bitwarden/sdk-go/v2)
    vault.go            # vault:// fetch — Vault API client
    cache.go            # AES-256-CBC encrypted cache with mlock'd memory
    uri.go              # Parse bw:// and vault:// URIs
  cleanup/              # Platform-specific cleanup hooks
    cleanup.go          # Interface + dispatcher
    cleanup_linux.go    # systemd inhibitor / logind D-Bus (sleep, lock)
    cleanup_darwin.go   # IOKit / NSWorkspace notifications
    cleanup_windows.go  # WTS session change / power events
  env/                  # .env file parsing and variable export/unload
    dotenv.go           # godotenv-based parser with URI-reference resolution
    yaml.go             # YAML file parsing with URI-reference resolution
    shell.go            # Shell integration: emit exports, track for unload
  kubeconfig/           # Kubeconfig merge, write, and KUBECONFIG management
    merge.go
    tmpfile.go          # RAM-backed tmpfile creation (cross-platform)
pkg/                    # (only if we need public library API — start with internal/)
Makefile                # Build, test, lint, install targets
flake.nix               # Nix dev shell + buildGoModule output
flake.lock
scripts/                # Helper scripts (goreleaser hooks, etc.)
docs/                   # Rewritten user-facing documentation for the Go tools
```

### Key design choices

- **Two binaries, one module** — `cmd/renv` and `cmd/kctx` share `internal/secrets` and `internal/cleanup`
- **`internal/` over `pkg/`** — start private; promote to `pkg/` only if external consumers appear
- **cobra** for CLI commands and flags
- **viper** for configuration management (config files, env vars, flag binding)
- **No global state** — pass context and config structs explicitly

---

## 5. Dependencies

| Library | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI command framework |
| `github.com/spf13/viper` | Configuration (files, env, flags) |
| `github.com/joho/godotenv` | `.env` file parsing |
| `gopkg.in/yaml.v3` | YAML parsing for `resolve_yaml_refs` replacement |
| `golang.org/x/sys/unix` | `Mlock`, platform syscalls |
| `github.com/hashicorp/vault/api` | Vault KV v2 client (or `vault-client-go`) |
| `github.com/bitwarden/sdk-go/v2` | Bitwarden Secrets Manager SDK (native Go API) |

Bitwarden integration uses the official Go SDK (`github.com/bitwarden/sdk-go/v2`) for native API access — no `bw` CLI subprocess dependency.

---

## 6. Security Model (Non-Negotiable)

These are hard constraints that must be maintained or improved in the rewrite:

1. **Secrets never touch disk** — use mlock'd memory or `/dev/shm` (Linux); equivalent OS mechanisms elsewhere
2. **Secrets are zeroed after use** — explicit `memset`-equivalent; no reliance on GC
3. **BW_SESSION is ephemeral** — unlock → fetch → zero the session token immediately
4. **Encrypted cache only** — AES-256-CBC + PBKDF2 with per-user, per-account keying
5. **Fail closed** — any retrieval failure is a hard error; never pass empty/partial credentials to child processes
6. **No secrets in shell history** — master password prompts use terminal raw mode or `gopass`-style masked input
7. **EXIT trap / cleanup hooks** — unset all exported variables and wipe cache on shell exit, sleep, lock
8. **`chmod 600`** — all written files (kubeconfig tmpfiles, cache) are owner-readable only

---

## 7. Implementation Order

```
Phase 1: Foundation
  ├── Initialize Go module, cobra root commands for renv and kctx
  ├── internal/secrets/uri.go — URI parsing (bw://, vault://)
  ├── internal/secrets/cache.go — mlock'd encrypted cache
  └── internal/secrets/bitwarden.go — BW CLI subprocess wrapper

Phase 2: renv core (issue #5)
  ├── internal/env/dotenv.go — .env parsing with URI resolution
  ├── internal/env/shell.go — export/unload/trap integration
  ├── cmd/renv/ — cobra commands: resolve, clear-cache, status
  └── Tests with mock BW CLI responses

Phase 3: renv YAML + Vault (issue #5 continued)
  ├── internal/env/yaml.go — YAML file URI resolution
  ├── internal/secrets/vault.go — Vault KV v2 fetch
  └── Tests with mock Vault responses

Phase 4: kctx (issue #6)
  ├── internal/kubeconfig/ — merge, tmpfile, KUBECONFIG management
  ├── cmd/kctx/ — cobra commands: switch, clear, status, cache-clear
  └── Shell integration snippet

Phase 5: Platform cleanup hooks (issue #1)
  ├── internal/cleanup/ — sleep, lock, shutdown hooks per platform
  ├── Wire into both renv and kctx
  └── Platform-specific tests

Phase 6: Polish
  ├── Collections support (issue #3)
  ├── Install scripts / goreleaser config
  ├── Updated documentation in docs/
  └── Backward compatibility validation against existing .env files
```

---

## 8. Build Tooling

### Makefile

A `Makefile` at the repo root handles all common tasks:

```makefile
# Key targets:

build           # Build both cmd/renv and cmd/kctx binaries to bin/
build-renv      # Build only renv
build-kctx      # Build only kctx
test            # Run all tests (go test ./...)
test-race       # Run tests with -race
test-cover      # Run tests with coverage report
lint            # Run staticcheck + go vet
fmt             # Run gofmt -w + goimports
tidy            # go mod tidy + go mod verify
clean           # Remove bin/ and build artifacts
install         # go install both binaries
release         # goreleaser build (snapshot, no publish)
```

Platform-specific build targets use `GOOS`/`GOARCH` env vars. CGO is required for the Bitwarden SDK (`sdk-go` uses cgo internally).

### flake.nix

The existing `flake.nix` will be rewritten for the Go-focused project. It provides:

- **Dev shell** — Go toolchain, `staticcheck`, `goimports`, `goreleaser`, `vault` CLI (for integration tests), `bitwarden-cli` (for integration tests), `jq`, `git`
- **CI reproducibility** — `nix develop` in CI gives identical tooling to local dev
- **Build output** — `nix build` produces the Go binaries directly (using `buildGoModule`)

Key packages in the dev shell:

```nix
# Go toolchain
pkgs.go
pkgs.gopls
pkgs.go-tools          # staticcheck
pkgs.gotools           # goimports
pkgs.goreleaser
pkgs.gnumake

# For integration testing
pkgs.vault
pkgs.bitwarden-cli

# Utilities
pkgs.git
pkgs.jq
```

---

## 9. Testing Strategy

- **Unit tests** for URI parsing, cache encryption/decryption, .env parsing, YAML resolution
- **Integration tests** with mock BW CLI (`os/exec` test doubles) and mock Vault server
- **Platform tests** via `//go:build` tagged test files for cleanup hooks
- **Backward compat tests** — run existing `.env` files from the repo through the new parser and verify identical output
- **Security tests** — verify mlock is applied, verify zeroing after use, verify fail-closed behavior

---

## 10. Out of Scope

- Re-adding `guides/`, `examples/`, `agents/`, `best-practices.md` — tracked as a separate issue
- GUI or TUI — CLI only