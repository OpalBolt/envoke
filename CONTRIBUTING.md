# Contributing to envoke

## Commit messages

Envoke uses [Conventional Commits](https://conventionalcommits.org/). Every commit on `main` is read by `release-please`, which uses the type prefix to decide whether to create a release and what kind of version bump to apply. **Your commit message is your vote on versioning.**

### Format

```
<type>(<scope>): <subject>

[optional body — explain *why*, not *what*]

[optional footer — breaking changes, issue refs]
```

### Types

| Type | When to use | Triggers a release? |
|------|-------------|---------------------|
| `feat` | New user-visible feature or capability | ✅ minor bump |
| `fix` | Bug fix visible to end users | ✅ patch bump |
| `perf` | Measurable performance improvement | ✅ patch bump |
| `revert` | Reverting a previous commit | ✅ patch bump |
| `refactor` | Code restructured, no user-facing change | ❌ |
| `docs` | Documentation only | ❌ |
| `test` | Adding or fixing tests | ❌ |
| `build` | Build system, `go.mod`, `flake.nix`, `Makefile` | ❌ |
| `ci` | GitHub Actions, CI configuration | ❌ |
| `chore` | Miscellaneous maintenance | ❌ |
| `style` | Formatting, whitespace — no logic change | ❌ |

### Scope (optional)

A lowercase word describing what part of the codebase changed.
Common scopes: `bw`, `vault`, `kctx`, `cache`, `config`, `cli`, `e2e`, `flake`.

```
fix(cache): prevent stale entries after bw unlock timeout
feat(vault): add KV v1 secret path support
ci: switch workflows to native go
```

### Breaking changes

Breaking changes trigger a **major version bump**. Mark them in one of two ways:

```
# Append ! after type/scope
feat!: remove --legacy-mode flag

# Or add a BREAKING CHANGE footer (blank line required)
feat(cli): redesign config file format

BREAKING CHANGE: config.yaml replaces .renv; existing files must be migrated
```

### Rules (enforced by CI)

- Type must be one of the allowed types above, lowercase
- Subject must not be empty
- Subject must not start with a capital letter
- Subject must not end with `.`
- Header (first line) max **100 characters**
- Body and footer lines max **100 characters**
- Body and footer must each be preceded by a blank line

### Good examples

```
feat(bw): support TOTP field in secret references
fix: handle empty vault token gracefully
refactor: extract secret URI parsing into its own package
docs: add nix flake usage to installation guide
ci: add commitlint check to PR workflow
chore: update govulncheck to latest
feat!: rename renv binary to envoke
```

### Bad examples

```
WIP                          ← no type
Fixed the bug                ← capital, no type, vague
feat: Added new feature.     ← capital subject, ends with period
feature: do stuff            ← 'feature' is not a valid type
```

### Subject line style

- **Imperative mood**: "add vault support" — not "added" or "adds"
- **What changed**, not how: "add KV v1 support" — not "update vault.go to call newEndpoint()"
- Keep it under 72 characters where possible (readable in `git log --oneline`)

---

## How versioning works

`release-please` runs on every push to `main`. It reads commit messages since the last release and maintains a **Release PR** that:

- Bumps `VERSION` according to the highest-impact commit (`fix` → patch, `feat` → minor, breaking → major)
- Generates `CHANGELOG.md` from `feat:` and `fix:` commits

When you merge the Release PR, a GitHub release is created and goreleaser builds the binaries. Nothing ships until a human merges that PR — you control the release cadence.
