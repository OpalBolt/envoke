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

## 4. Storing kubeconfig in Vault

For team environments, store kubeconfig in Vault rather than distributing files:

```bash
# Store
vault kv put secret/k8s/client-a-prod kubeconfig=@~/.kube/client-a-prod.kubeconfig

# Retrieve into temp file for a session
vault kv get -field=kubeconfig secret/k8s/client-a-prod > /tmp/kubeconfig-session
export KUBECONFIG=/tmp/kubeconfig-session

# Use it...
kubectl get pods

# Clean up
unset KUBECONFIG
rm /tmp/kubeconfig-session
```

---

## 5. Short-Lived Credentials with OIDC

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

## 6. Kubeconfig Merging

See [kubeconfig-merge.sh](../../snippets/kubeconfig-merge.sh) for a safe merging script.

Quick merge:

```bash
KUBECONFIG=~/.kube/config:~/.kube/new-cluster.kubeconfig \
  kubectl config view --flatten > ~/.kube/merged.config
```

Always review the merged file before trusting it:

```bash
kubectl config get-contexts --kubeconfig=~/.kube/merged.config
```

---

## 7. Offboarding / Credential Revocation

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
