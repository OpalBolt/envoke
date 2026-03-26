# Git Security — Preventing Secrets in Version Control

Secrets accidentally committed to git are one of the most common causes of credential leaks, even in private repositories. Once committed, a secret is in the git history and must be treated as compromised.

---

## 1. .gitignore

Add these patterns to `.gitignore` for every project:

```gitignore
# Environment files
.env
.env.*
!.env.example
!.env.template

# Secrets and credentials
*.pem
*.key
*.p12
*.pfx
*.crt
*.cer
*.der
id_rsa
id_ed25519
*.secret
secrets.yaml
secrets.json

# Cloud provider credentials
.aws/credentials
.aws/config
credentials.json
service-account*.json

# Kubernetes
kubeconfig
*.kubeconfig
kube-config

# Terraform
*.tfvars
!*.tfvars.example
terraform.tfstate
terraform.tfstate.backup
.terraform/

# Vault
.vault-token
```

---

## 2. Global Git Ignore

Set a global `.gitignore` that applies to all repos on your machine:

```bash
cat >> ~/.gitignore_global << 'EOF'
.env
.vault-token
*.pem
*.key
id_rsa
id_ed25519
*.tfvars
service-account*.json
EOF

git config --global core.excludesFile ~/.gitignore_global
```

---

## 3. Pre-commit Hooks with gitleaks

[gitleaks](https://github.com/gitleaks/gitleaks) scans for secrets in git history and staged files.

### Install gitleaks

```bash
# macOS
brew install gitleaks

# Linux (binary)
curl -sSfL https://github.com/gitleaks/gitleaks/releases/latest/download/gitleaks_$(uname -s)_$(uname -m).tar.gz | tar -xz
sudo mv gitleaks /usr/local/bin/
```

### Install as a pre-commit hook

Using the [pre-commit](https://pre-commit.com/) framework:

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.18.2
    hooks:
      - id: gitleaks
```

```bash
pip install pre-commit
pre-commit install
```

Now `git commit` will fail if gitleaks detects a secret.

### Manual scan

```bash
# Scan staged changes
gitleaks protect --staged

# Scan entire git history
gitleaks detect
```

---

## 4. Pre-commit Hooks with detect-secrets

[detect-secrets](https://github.com/Yelp/detect-secrets) from Yelp — pluggable, supports custom regex:

```bash
pip install detect-secrets
detect-secrets scan > .secrets.baseline
detect-secrets audit .secrets.baseline
```

Add to `.pre-commit-config.yaml`:

```yaml
  - repo: https://github.com/Yelp/detect-secrets
    rev: v1.4.0
    hooks:
      - id: detect-secrets
        args: ['--baseline', '.secrets.baseline']
```

---

## 5. If You Accidentally Commit a Secret

1. **Rotate the secret immediately** — assume it is compromised
2. **Remove from history** using `git-filter-repo`:
   ```bash
   pip install git-filter-repo
   git filter-repo --path-glob '*.env' --invert-paths
   # Force push all branches
   git push origin --all --force
   ```
3. **Alert your team** — anyone who cloned the repo may have the secret cached
4. **Check audit logs** — assume the secret was accessed

> ⚠️ Removing a secret from git history does not undo any access that occurred. Rotate first, clean history second.

---

## 6. Signed Commits (optional but recommended)

Sign commits with GPG to prove authorship:

```bash
gpg --full-generate-key
git config --global user.signingkey <KEY_ID>
git config --global commit.gpgsign true
```

---

## Related

- [Shell security](shell-security.md)
- [Pre-commit hook snippet](../../snippets/pre-commit-hook.sh)
