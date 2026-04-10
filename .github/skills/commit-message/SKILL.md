---
name: commit-message
description: >
  Enforce and write conventional commit messages for the envoke project.
  Use when writing a commit message, reviewing commits in a PR, or checking
  whether a commit follows the project standard.
---

# Commit Message Skill

Envoke uses **Conventional Commits** (`@commitlint/config-conventional`). Commit messages are not just documentation — they directly drive automated versioning via `release-please`:

| Commit type | Version impact |
|-------------|----------------|
| `fix:`, `perf:`, `revert:` | patch bump (`0.2.0 → 0.2.1`) |
| `feat:` | minor bump (`0.2.0 → 0.3.0`) |
| `feat!:` or `BREAKING CHANGE:` footer | major bump (`0.2.0 → 1.0.0`) |
| `refactor:`, `docs:`, `test:`, `build:`, `ci:`, `chore:`, `style:` | no release created |

## Format

```
<type>(<scope>): <subject>

[optional body]

[optional footer(s)]
```

All rules below are **errors** unless marked ⚠️ (warning).

## Rules

| Rule | Constraint |
|------|------------|
| `type` | Must be one of the allowed types (see below) |
| `type` | Lowercase only |
| `type` | Never empty |
| `subject` | Never empty |
| `subject` | Must not start with sentence-case, start-case, pascal-case, or upper-case |
| `subject` | Must not end with `.` |
| `header` | Max 100 characters total |
| `body` | Max 100 characters per line |
| `body` leading blank | ⚠️ Must have a blank line before body |
| `footer` | Max 100 characters per line |
| `footer` leading blank | ⚠️ Must have a blank line before footer |

## Allowed types

| Type | Use for |
|------|---------|
| `feat` | New user-visible feature or capability |
| `fix` | Bug fix visible to end users |
| `perf` | Performance improvement |
| `revert` | Reverting a previous commit |
| `refactor` | Code restructuring with no user-facing change |
| `docs` | Documentation only |
| `test` | Adding or fixing tests |
| `build` | Build system or dependency changes (`go.mod`, `flake.nix`, `Makefile`) |
| `ci` | CI/CD pipeline changes |
| `chore` | Miscellaneous maintenance (no production code change) |
| `style` | Code formatting, whitespace — no logic change |

## Scope (optional)

Lowercase. Recommended values: `bw`, `vault`, `kctx`, `cache`, `config`, `cli`, `e2e`, `flake`.

## Breaking changes

Two equivalent ways:

```
# Option 1 — append ! to type
feat!: remove --legacy-mode flag

# Option 2 — BREAKING CHANGE footer (blank line required before footer)
feat(cli): redesign config file format

BREAKING CHANGE: config.yaml replaces .renv; existing files must be migrated
```

## Subject line guidelines

- Use imperative mood: **"add X"** not "added X" or "adds X"
- Keep it ≤ 72 characters (well within the 100 limit, readable in `git log`)
- Describe *what* changed, not *how*

## Examples

**✅ Valid**
```
feat(vault): add KV v1 secret path support
fix(cache): prevent stale entries after bw unlock timeout
refactor: rename internal package secrets → providers
docs: update installation steps for macOS arm64
ci: switch from nix to native go in CI
chore: bump goreleaser to v2
feat!: drop support for bw CLI versions before 2023
```

**❌ Invalid**
```
WIP                               # missing type
Fix bug                           # capital letter, missing type
feat: Add new feature.            # capital subject, ends with period
feature: add thing                # 'feature' is not an allowed type
fix: some message that is over one hundred characters long and will fail the header-max-length rule
```

## When reviewing commit messages

1. Check type is in the allowed list and lowercase
2. Check subject is not empty, not capitalised at start, does not end with `.`
3. Check header is ≤ 100 characters
4. Check body/footer lines are ≤ 100 characters and separated by blank lines
5. Flag whether the commit will trigger a version bump (helps the author confirm intent)
