# Copilot Instructions

## Build, Test, and Lint

Go is not in `PATH` by default — use `nix develop` to enter the dev shell, or prefix commands with `nix shell nixpkgs#go nixpkgs#gotools --command`.

```bash
nix develop              # enter dev shell (Go, goreleaser, bw, vault, renv, kctx)
make build               # build renv and kctx → bin/
make test                # go test ./...
make test-race           # go test -race ./...  (used in CI)
make lint                # go vet ./...
make fmt                 # gofmt -w .
make tidy                # go mod tidy && go mod verify
```

Run a single test package or test function:
```bash
# inside nix develop (CGO_ENABLED=0 is already set by shellHook)
go test ./internal/secrets/...
go test -run TestParseBWRef ./internal/secrets/...
go test -race -run TestCacheRoundtrip ./internal/secrets/...
```

CI runs: `nix run .#test-race`, `nix run .#lint`, `nix run .#fmt-check`, `nix run .#shellcheck`.  
Shell scripts under `snippets/` are checked with `shellcheck --severity=warning`.

### Updating Go dependencies

After any `go.mod` change:
1. Set `vendorHash = pkgs.lib.fakeHash;` in `flake.nix` (`common` block)
2. Run `nix build` — it fails printing the correct hash
3. Replace with the reported `sha256-…` value

## Architecture

Two CLI binaries (`cmd/renv`, `cmd/kctx`) share the `internal/` packages. Both use [cobra](https://github.com/spf13/cobra) for subcommand dispatch and load config via `internal/config` in a `PersistentPreRunE` hook.

**Secret resolution flow (renv)**:
1. `env.ResolveDotEnv` parses `.env` → finds `bw://` / `vault://` values
2. `secrets.ParseBWRef` / `secrets.ParseVaultRef` parse the URIs
3. `BWClient.Resolve` or `VaultClient.Resolve` fetch the value
4. For Bitwarden: check encrypted cache → on miss, prompt passwords → run `bw unlock` / `bw list items` subprocess → encrypt result → write to `/dev/shm`
5. `env.EmitExports` writes `export KEY='value'` to stdout (shell-quoted, key validated against `^[A-Za-z_][A-Za-z0-9_]*$`)
6. Caller `eval`s the output; `renv init` installs a shell wrapper that does this automatically

**kctx flow**: fetch kubeconfig bytes from Vault or Bitwarden → `kubeconfig.WriteKubeconfig` writes an AES-encrypted tmpfile to `/dev/shm` with a `kctx-` prefix → emits `export KUBECONFIG=<path>` + `trap 'kctx clear' EXIT`.

**Internal packages**:
| Package | Purpose |
|---------|---------|
| `internal/secrets` | `BWClient`, `VaultClient`, `Cache` (AES-256-CBC), URI parsing (`BWRef`, `VaultRef`), var-name tracking |
| `internal/env` | `.env` parsing, YAML resolution, `EmitExports`/`EmitUnload` |
| `internal/config` | Shared `Config` struct; load order: defaults → YAML file → `RENV_*` env vars → CLI flags |
| `internal/kubeconfig` | Tmpfile write + kubeconfig merge |
| `internal/cleanup` | Platform-specific tmpdir helpers (Linux/macOS/Windows) |
| `internal/logger` | `slog`-based logger initialisation |
| `internal/version` | Version/commit/date strings injected via ldflags |

## Key Conventions

### Two-password model (Bitwarden)
`BWClient` uses two distinct passwords that must never be confused:
- **BWPassword** — Bitwarden master password, passed to `bw unlock` **via stdin** (`cmd.Stdin = bytes.NewBufferString(secret)`), never as CLI args, never persisted.
- **LocalPassword** — used only to encrypt/decrypt the `/dev/shm` cache (AES-256-CBC + PBKDF2-SHA256). Prompted once; optionally shared across terminals via a uid-keyed file in `/dev/shm` (disabled by `--isolated`).

### Secret reference URI formats
```
bw://folder/item                   # password field (default)
bw://folder/item/username
bw://folder/item/note
bw://folder/item/totp
bw://folder/item/field:custom_name
bw://collection:name/item
vault://secret/path#field          # KV v2; #field fragment required
```

### Shell output safety
`EmitExports` validates every key against `^[A-Za-z_][A-Za-z0-9_]*$` before writing. Values are single-quote escaped. This prevents injection when output is `eval`'d.

### Config loading order
Defaults → `$XDG_CONFIG_HOME/renv/config.yaml` → `RENV_*` env vars → CLI flags (applied by caller after `config.Load`). All duration strings use Go format (e.g. `"8h"`, `"30s"`).

### Cache
- Location: `/dev/shm` (Linux tmpfs) with `/tmp` fallback
- Encryption: AES-256-CBC; key = PBKDF2-SHA256(localPassword, salt, 100_000 iter, 32 bytes)
- Default TTL: 8 hours; `--no-cache` sets `Cache.Disabled = true` which makes `Put`/`Get` no-ops
- `cache.Clear` also called by `renv clear-cache` (triggered by shell EXIT trap); var-name tracking file is intentionally **not** cleared so `renv unload` still works after trap fires

### Managed-env detection
`renv resolve` skips emitting the EXIT trap when `DIRENV_DIR`, `DIRENV_FILE`, or `IN_NIX_SHELL` is set, since in those environments the process exits immediately after `.envrc` evaluation.

### Subcommand pattern
Each subcommand is a standalone `func xxxCmd(...) *cobra.Command` that closes over config/flag pointers passed from `rootCmd`. `newClients()` in `cmd/renv/main.go` is the single place that constructs `Cache`, `BWClient`, and `VaultClient`.

### Version injection
`internal/version` fields (`Version`, `Commit`, `BuildDate`) are empty strings at compile time and set by `-ldflags "-X ..."` in both `Makefile` and `flake.nix`. The `VERSION` file is the release source of truth.

### Environment variables

| Variable | Purpose |
|----------|---------|
| `RENV_BW_PASSWORD` | Bitwarden master password (non-interactive / CI) |
| `RENV_LOCAL_PASSWORD` | Local cache encryption password (skips prompt) |
| `BW_SESSION` | Pre-existing Bitwarden session token (skips `bw unlock`) |
| `VAULT_ADDR` | Vault server address |
| `VAULT_TOKEN` | Vault authentication token |
| `RENV_LOG_LEVEL` | `debug`/`info`/`warn`/`error` |
| `RENV_LOG_FORMAT` | `text` or `json` |
| `RENV_CACHE_MAX_AGE` | Cache TTL (Go duration, e.g. `"8h"`) |
| `RENV_ISOLATED` | `true`/`1` — per-terminal auth, no sharing |
| `RENV_PASSWORD_GRACE_PERIOD` | Re-prompt grace period (Go duration) |
| `RENV_TIMEOUT_BITWARDEN` | bw subprocess timeout |
| `RENV_TIMEOUT_VAULT` | vault subprocess timeout |

Cache files are named `renv-<uid>-<first16hex(SHA256(uid:acctTag:folder))>.enc` in `/dev/shm` (or `/tmp`).

### Coding conventions

- **Concrete types, not interfaces**: `BWClient` and `VaultClient` are structs, not interfaces — keep it that way unless a clear need arises.
- **Structured logging**: `slog.Debug("message", "key", value, ...)` throughout; never `fmt.Print` for diagnostics.
- **Best-effort cleanup**: operations that fail after a successful main action are logged with `slog.Warn` but don't return errors (e.g. cache writes after a successful Bitwarden fetch).
- **Errors via `%w`**: always wrap with context — `fmt.Errorf("resolving %s: %w", file, err)`.

### Snippets
`snippets/` contains standalone Bash/Python tools (not imported by Go):
- `resolve-env-refs/` — original Bash + Python implementation (with installer)
- `kctx.sh`, `kctx/` — standalone shell kctx helper
- `pre-commit-hook.sh` — checks for accidental secret commits (gitleaks or regex)
- `kubeconfig-merge.sh` — utility for merging kubeconfig files

Shell scripts must pass `shellcheck --severity=warning`.

---

## Code Review Agents

When reviewing pull requests, Copilot must apply **all four review agents** below. Each agent has a distinct focus. Run them independently and surface findings under a clearly labelled heading per agent.

---

### Agent: Security & Exploit Analysis

**Goal**: Find ways the new or changed code could be exploited, misused, or cause unintended secret disclosure.

Focus areas specific to this codebase:

- **Secret leakage** — Could a secret value end up in a log line, error message, stack trace, or temp file outside `/dev/shm`? Check `slog.*` calls near secret values. Check `fmt.Errorf` wrapping of errors that carry secret content.
- **Shell injection** — `EmitExports` eval-safety: are new env-key or env-value paths properly validated and single-quote-escaped before being written to stdout? Any new code that writes to a shell-eval'd stream must pass through the existing sanitisation pipeline.
- **URI/reference parsing** — New `bw://` or `vault://` URI parsers: do they reject path traversal (`../`), null bytes, or oversized inputs? Could a crafted URI cause a subprocess to receive unexpected arguments?
- **Subprocess argument injection** — Any call to `exec.Command` must not interpolate user-controlled strings directly into argument slices without validation. `bw` and `vault` CLI arguments must be checked.
- **Crypto downgrade / weaknesses** — New code touching the AES-256-CBC cache (PBKDF2-SHA256, 100k iterations) must not reduce iteration counts, shorten salt, or change the key-derivation path in a way that weakens stored secrets.
- **Password/credential handling** — Passwords must only flow via stdin to subprocesses, never via CLI flags, env vars printed to logs, or written to disk outside the encrypted cache. Check any new `cmd.Args` or `cmd.Env` usage.
- **Race conditions on shared state** — The `/dev/shm` password-sharing file is uid-keyed. New concurrent code paths must not introduce TOCTOU on that file. Check for missing locks.
- **Privilege / scope creep** — Does new code request more Bitwarden/Vault access than the referenced item requires? Is the minimum scope principle upheld?

Verdict: list each finding as `[CRITICAL]`, `[HIGH]`, `[MEDIUM]`, or `[INFO]`. If none found, state "No security issues found."

---

### Agent: Code Remnants & Cleanliness

**Goal**: Identify leftover development artifacts that must not ship.

Check for:

- `TODO`, `FIXME`, `HACK`, `XXX`, `TEMP`, `WIP` comments in new or changed lines.
- Commented-out code blocks (more than one line commented out consecutively is suspicious).
- Hardcoded credentials, tokens, UUIDs, IP addresses, or hostnames (anything that looks like a real value rather than a placeholder).
- Debug `fmt.Print*` / `log.Print*` statements — all diagnostic output must go through `slog`.
- Leftover test helpers, `t.Skip(...)` with no explanation, or `_ = someVar` used to silence unused-variable errors in production code.
- Dead imports (`_ "pkg"` in non-test files that have no clear side-effect justification).
- Unused exported symbols added in this PR that have no callers yet.
- Files that belong in `snippets/` (standalone scripts) accidentally placed in `internal/` or `cmd/`.

Verdict: list each finding with file + line. If none found, state "No remnants found."

---

### Agent: Coding Standards Compliance

**Goal**: Verify the PR follows this project's conventions exactly as documented in these instructions.

Check every changed `.go` file against:

1. **Concrete types, not interfaces** — `BWClient`, `VaultClient`, and provider structs must remain concrete. New abstractions need explicit justification.
2. **Error wrapping** — Every `return err` without context is a violation. Must be `fmt.Errorf("doing X: %w", err)`.
3. **Structured logging** — `slog.Debug/Info/Warn/Error("message", "key", value)` only. No `fmt.Printf` for diagnostics, no bare `log.Println`.
4. **Best-effort cleanup** — Post-success cleanup failures (cache writes, temp-file removal) must be logged with `slog.Warn`, not returned as errors.
5. **Two-password model integrity** — `BWPassword` and `LocalPassword` must never be mixed up. Passwords passed to subprocesses go via `cmd.Stdin` only.
6. **Shell output safety** — Keys emitted via `EmitExports` must pass the `^[A-Za-z_][A-Za-z0-9_]*$` regex check. Values must be single-quote escaped.
7. **Subcommand pattern** — New commands follow the `func xxxCmd(...) *cobra.Command` pattern closing over config/flag pointers. No global state.
8. **Test coverage** — New exported functions and non-trivial logic paths must have at least one table-driven test. Tests use `t.Parallel()` where safe.
9. **`go vet` / `gofmt` cleanliness** — No formatting drift; CI enforces this but flag it if obvious.

Verdict: list each violation with file + line + rule number above. If none found, state "Standards compliant."

---

### Agent: Test Quality & Coverage

**Goal**: Ensure new logic is adequately tested and tests are trustworthy.

Check for:

- New exported functions or non-trivial code paths with **no corresponding test**.
- Tests that only test the happy path — are error paths and edge cases (empty input, malformed URIs, cache miss, subprocess failure) covered?
- Tests that mock or stub in ways that make them pass trivially without exercising real logic.
- Race-condition-prone tests: shared state, missing `t.Parallel()`, or missing `-race` compatibility.
- Tests that read from real `/dev/shm`, real Bitwarden, or real Vault without a clear skip guard (`t.Skipf("requires BW_SESSION")` etc.).
- Assert quality: vague `if err != nil { t.Fatal }` without checking the specific error value when the specific error matters.
- Benchmark additions that are missing a baseline comparison or don't call `b.ReportAllocs()`.

Verdict: list gaps as `[MISSING TEST]` or `[WEAK TEST]` with function/file. If coverage is adequate, state "Test coverage adequate."
