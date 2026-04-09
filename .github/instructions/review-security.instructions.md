---
applyTo: "**"
---

# Review Agent: Security & Exploit Analysis

**Trigger**: Apply on every pull request review.

**Goal**: Find ways the new or changed code could be exploited, misused, or cause unintended secret disclosure. This codebase handles real user secrets — findings here are high priority.

## Checks

### Secret leakage
- Could a secret value end up in a log line, error message, stack trace, or temp file outside `/dev/shm`?
- Inspect every `slog.*` call near secret values. Field values must never contain raw passwords, BW session tokens, or Vault tokens.
- Check `fmt.Errorf` wrapping: does the wrapped error carry secret content into caller log output?

### Shell injection via eval
- `EmitExports` output is `eval`'d by the calling shell. Every new code path that writes to that stream must pass through the existing key-validation (`^[A-Za-z_][A-Za-z0-9_]*$`) and single-quote value escaping.
- Any bypass or shortcut around those checks is a critical shell-injection vector.

### URI / reference parsing
- New `bw://` or `vault://` URI parsers must reject: path traversal (`../`), null bytes, and inputs exceeding reasonable length limits.
- A crafted URI must not cause a subprocess to receive unexpected positional arguments or flags.

### Subprocess argument injection
- Every `exec.Command` call must use a fixed argument slice — no `fmt.Sprintf` or string concatenation of user-controlled values into `cmd.Args`.
- `bw` and `vault` CLI arguments must be validated before use.
- Passwords flow via `cmd.Stdin` only — never in `cmd.Args` or `cmd.Env`.

### Crypto weaknesses
- New code touching the AES-256-CBC cache (PBKDF2-SHA256, 100k iterations) must not reduce iteration count, shorten the salt, or change the KDF in a way that weakens stored secrets.
- No new use of MD5, SHA-1, DES, RC4, or `math/rand` for security-sensitive operations.

### Race conditions on shared state
- The `/dev/shm` password-sharing file is uid-keyed. New concurrent code paths must not introduce TOCTOU vulnerabilities on that file.
- Check for missing mutex/lock protection around shared in-memory state.

### Privilege / scope creep
- Does new code request broader Bitwarden collection or Vault path access than the referenced item requires?
- Is the minimum-scope principle upheld in any new `bw` or `vault` invocation?

## Output format

List each finding as:

```
[SEVERITY] file:line — description
```

Severity levels: `[CRITICAL]`, `[HIGH]`, `[MEDIUM]`, `[INFO]`

If no issues are found, state: **"No security issues found."**
