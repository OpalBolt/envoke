# Secure Handling of Secrets

A collection of guides, code snippets, and tools for consultants to securely manage secrets when working locally — covering API keys, tokens, passwords, kubectl config files, certificates, and other sensitive information.

## Problem Statement

Consultants regularly handle sensitive credentials across multiple projects and clients. Without clear guidance and tooling, secrets end up in plaintext files, shell histories, environment variables in dotfiles, or accidentally committed to version control. This creates security and compliance risks.

## Goals

- Provide **practical, copy-paste-ready guides** for securely storing and retrieving secrets locally
- Standardise on **HashiCorp Vault** and **Bitwarden** as approved secret backends
- Reduce risk of accidental secret exposure (e.g. git commits, shell history, log files)
- Follow industry best practices for encryption, access control, and auditing
- Offer **AI agents** to automate common secret management tasks

## Supported Secret Backends

| Backend | Use Case |
|---|---|
| **HashiCorp Vault** | Team/project secrets, dynamic credentials, short-lived tokens |
| **Bitwarden** | Personal credentials, API keys, passwords |

## Scope of Secrets

| Secret Type | Examples |
|---|---|
| API keys & tokens | Cloud provider keys, SaaS API tokens, OAuth tokens |
| Passwords | Database credentials, service accounts |
| Kubernetes config | kubectl config files, service account tokens |
| Certificates & keys | TLS certs, SSH keys, GPG keys |
| Environment config | `.env` files with sensitive values |

## Project Structure

```
.
├── readme.md
├── guides/
│   ├── vault/
│   │   ├── setup.md              # Installing and configuring Vault CLI
│   │   ├── authentication.md     # Auth methods (token, OIDC, AppRole)
│   │   ├── read-write-secrets.md # Storing and retrieving secrets
│   │   └── dynamic-secrets.md    # Short-lived database/cloud credentials
│   ├── bitwarden/
│   │   ├── setup.md              # Installing Bitwarden CLI
│   │   ├── usage.md              # Storing and retrieving secrets
│   │   └── scripting.md          # Using Bitwarden CLI in scripts
│   ├── general/
│   │   ├── git-security.md       # Preventing secrets in git (pre-commit hooks, .gitignore)
│   │   ├── env-files.md          # Secure handling of .env files
│   │   ├── shell-security.md     # Avoiding secrets in shell history and logs
│   │   └── secret-rotation.md    # Rotation practices and reminders
│   └── kubernetes/
│       ├── kubeconfig.md         # Secure kubeconfig management
│       └── k8s-secrets.md        # Working with Kubernetes secrets locally
├── snippets/
│   ├── resolve-env-refs.sh       # Resolve bw:// and vault:// refs; self-loading .env + direnv
│   ├── pre-commit-hook.sh        # Git pre-commit hook to detect secrets
│   └── kubeconfig-merge.sh       # Safely merge kubeconfig files
├── examples/
│   ├── python/
│   │   ├── vault_client.py       # Read/write secrets with hvac
│   │   ├── bitwarden_client.py   # Retrieve secrets via Bitwarden CLI
│   │   └── requirements.txt      # Python dependencies
│   ├── go/
│   │   ├── vault_client.go       # Read/write secrets with Vault SDK
│   │   ├── bitwarden_client.go   # Retrieve secrets via Bitwarden CLI
│   │   └── go.mod                # Go module definition
│   ├── typescript/
│   │   ├── vault-client.ts       # Read/write secrets with node-vault
│   │   ├── bitwarden-client.ts   # Retrieve secrets via Bitwarden CLI
│   │   ├── package.json          # Node dependencies
│   │   └── tsconfig.json         # TypeScript config
│   └── bash/
│       ├── vault-client.sh       # Read/write secrets with Vault CLI
│       └── bitwarden-client.sh   # Retrieve secrets via Bitwarden CLI
├── agents/
│   ├── README.md                 # Overview of available AI agents
│   ├── secret-scanner/           # Agent: scan for leaked secrets in local repos
│   └── secret-injector/          # Agent: inject secrets from Vault/Bitwarden into env
└── best-practices.md             # Summary of do's and don'ts
```

## Best Practices (Summary)

### Do

- Store secrets in Vault or Bitwarden — never in plaintext files
- Use short-lived / dynamic credentials where possible
- Use `env` variables injected at runtime, not baked into files
- Use `.gitignore` and pre-commit hooks (e.g. `gitleaks`, `detect-secrets`) to prevent accidental commits
- Lock your Bitwarden vault when not in use
- Rotate secrets on a regular schedule

### Don't

- Commit secrets to version control (even private repos)
- Store secrets in shell history (`HISTIGNORE`, `HISTCONTROL`, or prefix commands with a space)
- Put secrets in Dockerfiles, CI config, or build logs
- Share secrets over Slack, email, or other unencrypted channels
- Use the same secret across multiple projects or clients

## Getting Started

1. **Choose your backend** — determine whether your project uses HashiCorp Vault, Bitwarden, or both
2. **Follow the setup guide** — see `guides/vault/setup.md` or `guides/bitwarden/setup.md`
3. **Install pre-commit hooks** — see `guides/general/git-security.md`
4. **Use the snippets** — copy and adapt the shell snippets from `snippets/`

## Contributing

This is a living resource. To contribute:

1. Create a branch from `main`
2. Add or update guides/snippets following the existing structure
3. Ensure no real secrets are included in examples (use placeholders like `<YOUR_TOKEN>`)
4. Open a pull request for review