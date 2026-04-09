---
applyTo: "**/*.go"
---

# Review Agent: Coding Standards Compliance

**Trigger**: Apply on every pull request review that changes `.go` files.

**Goal**: Verify the PR follows this project's conventions exactly. These rules are non-negotiable — flag every violation.

## Rules

### 1. Concrete types, not interfaces
`BWClient`, `VaultClient`, and provider structs must remain concrete structs.
New interfaces require explicit justification in the PR description.
Flag any `type XxxClient interface { ... }` added without that justification.

### 2. Error wrapping with context
Every `return err` without context is a violation.
All errors must be wrapped: `fmt.Errorf("doing X: %w", err)`.
Bare `return err` after a meaningful operation is always wrong.

### 3. Structured logging only
- `slog.Debug/Info/Warn/Error("message", "key", value, ...)` — correct.
- `fmt.Printf`, `fmt.Println`, `log.Println`, `log.Printf` in non-test code — violation.
- Log calls must include relevant key/value context, not just a bare message string.

### 4. Best-effort cleanup
Operations that fail *after* a successful main action (cache writes, temp-file removal, session cleanup) must:
- Be logged with `slog.Warn("...", "err", err)`.
- Not be returned as errors to the caller.
Returning a cleanup error that masks a successful result is a violation.

### 5. Two-password model integrity
- `BWPassword` (Bitwarden master password) and `LocalPassword` (cache encryption key) must never be mixed up, swapped, or used interchangeably.
- Passwords passed to subprocesses must go via `cmd.Stdin` only — never in `cmd.Args`, `cmd.Env`, or log output.
- New code that touches either password field must be scrutinised carefully.

### 6. Shell output safety
- Keys written by `EmitExports` must pass the `^[A-Za-z_][A-Za-z0-9_]*$` regex check before output.
- Values must be single-quote escaped before being written to the eval stream.
- Any new code that writes to the exports stream bypassing these checks is a critical violation.

### 7. Subcommand pattern
- New commands follow `func xxxCmd(...) *cobra.Command` closing over config/flag pointers.
- No package-level global variables for command state.
- `newClients()` in `cmd/renv/main.go` (or its equivalent) remains the single construction point for `Cache`, `BWClient`, and `VaultClient`.

### 8. Test coverage for new logic
- Every new exported function must have at least one test.
- Non-trivial logic paths (error branches, edge cases) must have corresponding test cases.
- Table-driven tests (`[]struct{ ... }`) are the preferred pattern.
- Tests that can safely run concurrently must call `t.Parallel()`.

### 9. Formatting and vet
- Code must be `gofmt`-clean (CI enforces this, but flag obvious drift).
- No `go vet` violations — shadowed variables, unreachable code, incorrect `Printf` format strings.

## Output format

List each violation as:

```
file:line — Rule N: description
```

If the PR is fully compliant, state: **"Standards compliant."**
