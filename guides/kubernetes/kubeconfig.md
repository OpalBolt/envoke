# Secure kubeconfig Management

`kubeconfig` files contain credentials for Kubernetes clusters. They must be treated as sensitive secrets.

---

## The Risk

A kubeconfig file typically contains:
- Cluster API server address
- CA certificate
- Client certificate or bearer token (often cluster-admin level)

Anyone with your kubeconfig has the same access level as you to the cluster.

---

## 1. Default kubeconfig Location

```
~/.kube/config
```

Protect file permissions:

```bash
chmod 600 ~/.kube/config
```

Never commit to git. Add to `.gitignore`:

```bash
echo "kubeconfig" >> ~/.gitignore_global
echo "*.kubeconfig" >> ~/.gitignore_global
echo ".kube/" >> ~/.gitignore_global
```

---

## 2. Multiple Clusters — Use KUBECONFIG Variable

Avoid merging all clusters into a single file. Keep per-client configs separate:

```bash
# Use a specific kubeconfig for a session
export KUBECONFIG=~/.kube/client-a-prod.kubeconfig

# Or scope to a single command
KUBECONFIG=~/.kube/client-a-prod.kubeconfig kubectl get pods

# List all contexts across multiple files
KUBECONFIG=~/.kube/client-a-prod.kubeconfig:~/.kube/client-b-staging.kubeconfig kubectl config get-contexts
```

---

## 3. Context Switching

```bash
# List contexts
kubectl config get-contexts

# Switch context
kubectl config use-context <CONTEXT_NAME>

# View current context
kubectl config current-context

# Run a command in a specific context without switching
kubectl --context=<CONTEXT_NAME> get pods
```

Install `kubectx` and `kubens` for fast context and namespace switching:

```bash
brew install kubectx
kubectx             # list contexts
kubectx <context>   # switch context
kubens <namespace>  # switch namespace
```

---

## 4. Never Writing to Disk

`chmod 600` limits who can read a file, but the file is still on disk — accessible to root, included in backups, and readable by any process running as your user. For genuinely sensitive cluster credentials, avoid disk entirely.

### Process substitution (bash / zsh)

Fetch the kubeconfig from Vault and pipe it directly to `kubectl` without writing a file:

```bash
# Single command — kubeconfig never touches disk
KUBECONFIG=<(vault kv get -field=kubeconfig secret/k8s/client-a-prod) kubectl get pods

# Wrap in a shell function for convenience
kube_client_a() {
  KUBECONFIG=<(vault kv get -field=kubeconfig secret/k8s/client-a-prod) kubectl "$@"
}

kube_client_a get pods
kube_client_a apply -f deployment.yaml
```

> ⚠️ Process substitution creates a file descriptor, not a real file. It is not compatible with tools that require a file path (e.g., some IDE integrations). Use the tmpfs approach below in those cases.

### tmpfs mount (in-memory filesystem)

Mount a RAM-based filesystem at `~/.kube`. Files written there never touch the physical disk and disappear on reboot:

```bash
# Mount a tmpfs at ~/.kube, owned by your user
sudo mount -t tmpfs -o size=10m,mode=0700,uid=$(id -u),gid=$(id -g) tmpfs ~/.kube

# Now write kubeconfig normally — it lives in RAM only
vault kv get -field=kubeconfig secret/k8s/client-a-prod > ~/.kube/config
chmod 600 ~/.kube/config

# Verify it's mounted
mount | grep kube

# Unmount and wipe when done
sudo umount ~/.kube
```

> ℹ️ On macOS, use a RAM disk: `diskutil eraseDisk APFS ramdisk $(hdiutil attach -nomount ram://20480)` then symlink `~/.kube` to the mount point.

### Use OIDC so the kubeconfig has no secrets

This is the gold standard: configure clusters to use OIDC authentication. The kubeconfig then contains only the cluster URL and OIDC issuer — no token, no certificate. It is safe to store anywhere.

```bash
# The kubeconfig only contains public metadata — the OIDC provider handles auth
kubectl config set-credentials <USER> \
  --auth-provider=oidc \
  --auth-provider-arg=idp-issuer-url=https://sso.example.com \
  --auth-provider-arg=client-id=<CLIENT_ID>
  # No client-secret needed for public clients (PKCE flow)
```

When `kubectl` needs credentials it redirects to the OIDC provider, obtaining a short-lived token at runtime. The kubeconfig file itself is non-sensitive.

### exec credential plugin

For clusters using tools like `kubelogin`, `aws eks get-token`, or `gke-gcloud-auth-plugin`, the kubeconfig delegates credential fetching to an external command:

```yaml
users:
  - name: my-cluster
    user:
      exec:
        apiVersion: client.authentication.k8s.io/v1beta1
        command: kubelogin
        args: ["get-token", "--environment", "AzurePublicCloud", "--server-id", "<APP_ID>"]
```

The kubeconfig file has no static credentials — tokens are fetched dynamically. The kubeconfig is non-sensitive by itself, though it still reveals cluster endpoint information; follow your organisation's policy on whether it can be shared.

---

## 5. Storing kubeconfig in Vault

For team environments, store kubeconfig in Vault rather than distributing files:

```bash
# Store
vault kv put secret/k8s/client-a-prod kubeconfig=@~/.kube/client-a-prod.kubeconfig

# Retrieve without touching disk (process substitution)
KUBECONFIG=<(vault kv get -field=kubeconfig secret/k8s/client-a-prod) kubectl get pods

# Or retrieve into a tmpfs-backed path for tools that need a file path
vault kv get -field=kubeconfig secret/k8s/client-a-prod > ~/.kube/config  # only if ~/.kube is on tmpfs
```

---

## 6. Short-Lived Credentials with OIDC

Configure kubectl to use OIDC authentication (your organisation's SSO) instead of long-lived client certificates:

```bash
kubectl config set-credentials <USER> \
  --auth-provider=oidc \
  --auth-provider-arg=idp-issuer-url=https://accounts.google.com \
  --auth-provider-arg=client-id=<CLIENT_ID> \
  --auth-provider-arg=client-secret=<CLIENT_SECRET>
```

This means the kubeconfig itself doesn't contain a long-lived token — the OIDC provider handles authentication.

---

## 7. Kubeconfig Merging

See [kubeconfig-merge.sh](../../snippets/kubeconfig-merge.sh) for a safe merging script with conflict detection and automatic backup. Usage:

```bash
# Preview first (dry run)
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig --dry-run

# Merge (creates a timestamped backup automatically)
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig
```

Quick merge (manual):

```bash
KUBECONFIG=~/.kube/config:~/.kube/new-cluster.kubeconfig \
  kubectl config view --flatten > ~/.kube/merged.config
```

Always review the merged file before trusting it:

```bash
kubectl config get-contexts --kubeconfig=~/.kube/merged.config
```

---

## 8. Offboarding / Credential Revocation

When a project ends:

```bash
# Delete the kubeconfig
rm ~/.kube/client-a-prod.kubeconfig

# Revoke your service account token (if applicable)
kubectl delete serviceaccount <NAME> -n <NAMESPACE>
```

Ask the cluster admin to revoke your OIDC/certificate credentials server-side.

---

## Related

- [Kubernetes Secrets](k8s-secrets.md)
- [kubeconfig-merge.sh snippet](../../snippets/kubeconfig-merge.sh)
- [snippets/README.md — how to use snippets](../../snippets/README.md)
