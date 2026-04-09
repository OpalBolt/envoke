---
applyTo: "**"
---

# Review Agent: Code Remnants & Cleanliness

**Trigger**: Apply on every pull request review.

**Goal**: Identify leftover development artifacts that must not ship to production.

## Checks

### Development comments
- `TODO`, `FIXME`, `HACK`, `XXX`, `TEMP`, `WIP` comments in new or changed lines.
- Any comment that reads like a note-to-self rather than useful documentation.

### Commented-out code
- More than one consecutive line of commented-out code is a flag — it should be deleted, not committed.
- Exception: short commented examples in documentation blocks are acceptable.

### Hardcoded values
- Credentials, tokens, API keys, UUIDs, IP addresses, or hostnames that look like real values rather than placeholders (`example.com`, `localhost`, `127.0.0.1` are fine).
- Any string that resembles a real Bitwarden item path, Vault path, or kubeconfig server URL.

### Debug output
- `fmt.Print*`, `fmt.Fprint*`, `log.Print*`, `log.Fatal*` in non-test production code — all diagnostic output must use `slog`.
- Temporary `os.Stderr.WriteString(...)` or similar one-off debug writes.

### Test leftovers in production code
- `t.Skip(...)` with no explanatory comment.
- `_ = someVar` used purely to silence an unused-variable compile error in production (not test) code.
- Test helper functions placed in non-`_test.go` files without a clear reason.

### Dead code
- Unused exported symbols added in this PR with no callers anywhere in the module.
- Dead imports: `_ "pkg"` in non-test files with no documented side-effect justification.

### Misplaced files
- Standalone scripts or shell utilities accidentally placed in `internal/` or `cmd/` instead of `snippets/`.
- Binary or generated files committed that belong in `.gitignore`.

## Output format

List each finding as:

```
file:line — description
```

If no remnants are found, state: **"No remnants found."**
