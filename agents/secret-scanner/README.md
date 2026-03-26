# Secret Scanner Agent

An AI agent that scans git repositories for leaked or hardcoded secrets.

---

## What It Does

- Scans staged changes, working tree, or full git history
- Uses `gitleaks` (if installed) as the primary scanner
- Falls back to regex pattern matching for common secret formats
- Reports findings with file, line, and pattern details
- Exits non-zero if secrets are found (suitable for CI/CD gates)

---

## Prerequisites

- `git` installed and in PATH
- `gitleaks` (optional but recommended): `brew install gitleaks`
- `jq` for JSON output formatting

---

## Usage

```bash
# Scan staged changes (pre-commit mode)
./scan.sh --staged

# Scan the entire working tree
./scan.sh

# Scan full git history
./scan.sh --history

# Output results as JSON
./scan.sh --format json

# Exit 0 even if secrets found (audit mode)
./scan.sh --audit
```

---

## Integration as Pre-commit Hook

```bash
cp snippets/pre-commit-hook.sh .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

Or with the pre-commit framework:

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.18.2
    hooks:
      - id: gitleaks
```

---

## Detected Patterns

| Pattern | Examples |
|---|---|
| AWS access keys | `AKIA...` |
| Stripe API keys | `sk_live_...`, `sk_test_...` |
| GitHub tokens | `ghp_...`, `github_pat_...` |
| GitLab tokens | `glpat-...` |
| Slack tokens | `xoxb-...`, `xoxp-...` |
| JWT tokens | `eyJ...` |
| Private keys | `BEGIN RSA/EC/OPENSSH PRIVATE KEY` |
| Hardcoded passwords | `password = "..."` assignments |
| Hardcoded API keys | `api_key = "..."` assignments |
| `.env` files | Any staged `.env` file |

---

## False Positives

To mark a line as a false positive (e.g., in test fixtures):

```bash
# Add to .gitleaks.toml
[[allowlist.regexes]]
description = "Test fixture — not a real key"
regex = "AKIAIOSFODNN7EXAMPLE"
```

Or use the `gitleaks` baseline:

```bash
gitleaks detect --report-path .gitleaks-baseline.json
gitleaks protect --baseline-path .gitleaks-baseline.json
```
