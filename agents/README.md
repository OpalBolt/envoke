# AI Agents — Secure Secret Management

This directory contains AI agent definitions and supporting scripts for automating common secret management tasks.

---

## Available Agents

### [`secret-scanner/`](secret-scanner/)

**Purpose**: Scan local git repositories for leaked or hardcoded secrets.

**What it does**:
- Scans staged changes, working tree, and git history
- Uses gitleaks and pattern matching to detect credentials
- Reports findings with file paths, line numbers, and matched patterns
- Integrates as a pre-commit hook or standalone CLI

**When to use**:
- Before pushing a commit to a remote
- Periodically auditing an existing repository
- Onboarding a new project where secrets may have been committed historically

**See**: [`secret-scanner/README.md`](secret-scanner/README.md)

---

### [`secret-injector/`](secret-injector/)

**Purpose**: Inject secrets from Vault or Bitwarden into the environment of a process.

**What it does**:
- Reads secrets from Vault KV or Bitwarden CLI
- Injects them as environment variables into a child process
- Ensures secrets are never written to disk or shell history
- Supports both interactive and unattended (CI/CD) modes

**When to use**:
- Starting a local development server that needs credentials
- Running one-off scripts that require API keys
- CI/CD pipelines where secrets must be injected at runtime

**See**: [`secret-injector/README.md`](secret-injector/README.md)

---

## Using Agents with GitHub Copilot CLI

These agent definitions follow the GitHub Copilot CLI agent format. To invoke an agent:

```bash
# Scan current repo for secrets
gh copilot agent secret-scanner scan

# Inject secrets and run a command
gh copilot agent secret-injector run vault secret/myproject/dev -- node server.js
```

---

## Design Principles

- **No secret persistence**: Agents never write secrets to disk
- **Minimal permissions**: Agents request only the paths/items they need
- **Auditability**: All secret retrievals are logged (Vault audit log, Bitwarden event log)
- **Fail closed**: If secret retrieval fails, the agent exits non-zero rather than continuing with empty values
