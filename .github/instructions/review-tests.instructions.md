---
applyTo: "**"
---

# Review Agent: Test Quality & Coverage

**Trigger**: Apply on every pull request review.

**Goal**: Ensure new logic is adequately tested and that tests are trustworthy. A test that passes trivially without exercising real logic is worse than no test.

## Checks

### Missing tests
- New exported functions with no corresponding test file entry.
- New non-trivial logic paths (conditional branches, error handling, loops with side effects) that have no test coverage.
- New CLI subcommands with no integration-style test or at least a unit test for the core logic they invoke.

### Weak or superficial tests
- Tests that only exercise the happy path — are error paths and edge cases covered?
  - Empty input, malformed URIs, cache miss, subprocess failure, context cancellation.
- Tests that mock or stub so heavily that they don't exercise real logic (e.g. a `BWClient` test that stubs out every method).
- Assertions that check `err != nil` without verifying the specific error type or message when the specific error matters.
- Tests that assert on output strings using `strings.Contains` for messages that should be exact.

### Race-unsafe tests
- Shared mutable state accessed from multiple goroutines without synchronisation.
- Missing `t.Parallel()` on tests that are safe to parallelise and that interact with unshared resources.
- Tests that write to fixed file paths in `/tmp` without using `t.TempDir()` — these race when run in parallel.

### Real-resource access without skip guards
- Tests that read from or write to real `/dev/shm` paths (outside `t.TempDir()` equivalents) without a skip guard.
- Tests that call real Bitwarden (`bw` CLI) or real Vault without:
  ```go
  if os.Getenv("BW_SESSION") == "" {
      t.Skip("requires BW_SESSION")
  }
  ```
- Tests that make real network calls without a build tag or skip guard.

### Benchmark quality
- New benchmarks that don't call `b.ResetTimer()` after setup.
- Benchmarks that don't call `b.ReportAllocs()` for memory-sensitive paths.
- Benchmarks without a meaningful baseline comparison noted in a comment.

### Test helper hygiene
- Test helpers that call `t.Fatal` directly instead of accepting `testing.TB` (limits reuse in subtests).
- Helper functions in `_test.go` files that duplicate existing helpers elsewhere in the test suite.

## Output format

List each finding as:

```
[MISSING TEST] or [WEAK TEST] — file/function — description
```

If test coverage is adequate, state: **"Test coverage adequate."**
