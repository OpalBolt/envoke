# Working with Kubernetes Secrets Locally

Kubernetes Secrets store sensitive data (passwords, tokens, keys) as base64-encoded values within the cluster. This guide covers how to work with them safely from a local workstation.

---

## Kubernetes Secrets Are Not Encrypted by Default

Base64 is encoding, not encryption. Anyone with access to the namespace can read Secrets.

```bash
kubectl get secret my-secret -o jsonpath='{.data.password}' | base64 -d
```

Ensure your cluster has **encryption at rest** enabled — ask your cluster admin.

---

## 1. Reading Secrets

```bash
# List secrets in a namespace
kubectl get secrets -n <NAMESPACE>

# Describe (shows metadata, not values)
kubectl describe secret <SECRET_NAME> -n <NAMESPACE>

# Get a specific field (base64-encoded)
kubectl get secret <SECRET_NAME> -n <NAMESPACE> -o jsonpath='{.data.<KEY>}'

# Decode immediately
kubectl get secret <SECRET_NAME> -n <NAMESPACE> -o jsonpath='{.data.<KEY>}' | base64 -d

# Get all decoded data
kubectl get secret <SECRET_NAME> -n <NAMESPACE> -o json | \
  jq '.data | map_values(@base64d)'
```

---

## 2. Creating Secrets

### From literal values

```bash
kubectl create secret generic db-credentials \
  --from-literal=username=app_user \
  --from-literal=password="<YOUR_PASSWORD>" \
  -n <NAMESPACE>
```

### From a file

```bash
kubectl create secret generic tls-cert \
  --from-file=tls.crt=./server.crt \
  --from-file=tls.key=./server.key \
  -n <NAMESPACE>
```

### From a Vault-injected value

```bash
DB_PASSWORD=$(vault kv get -field=password secret/myproject/db)

kubectl create secret generic db-credentials \
  --from-literal=password="$DB_PASSWORD" \
  -n <NAMESPACE>

unset DB_PASSWORD
```

---

## 3. Updating Secrets

```bash
# Update a single key
kubectl patch secret <SECRET_NAME> -n <NAMESPACE> \
  --type='json' \
  -p='[{"op":"replace","path":"/data/password","value":"'"$(echo -n '<NEW_PASSWORD>' | base64)"'"}]'
```

Or delete and recreate:

```bash
kubectl delete secret <SECRET_NAME> -n <NAMESPACE>
kubectl create secret generic <SECRET_NAME> --from-literal=password="<NEW_PASSWORD>" -n <NAMESPACE>
```

---

## 4. External Secrets Operator (recommended for production)

Rather than managing K8s secrets manually, use the [External Secrets Operator](https://external-secrets.io/) to sync from Vault automatically:

```yaml
# ExternalSecret pointing to Vault
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: db-credentials
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: db-credentials
    creationPolicy: Owner
  data:
    - secretKey: password
      remoteRef:
        key: secret/myproject/db
        property: password
```

This keeps secrets managed in Vault and synced automatically — no manual `kubectl create secret` needed.

---

## 5. Vault Agent Injector

The Vault Agent Injector injects secrets directly into pods as files or environment variables without requiring application code changes:

```yaml
# Pod annotation
annotations:
  vault.hashicorp.com/agent-inject: "true"
  vault.hashicorp.com/role: "myapp"
  vault.hashicorp.com/agent-inject-secret-config: "secret/data/myproject/db"
```

---

## 6. Avoiding Secret Sprawl

- Do not copy secrets from Kubernetes to local files
- Do not use `kubectl cp` to transfer secrets to local disk
- Review who has `get`/`list` on Secrets: `kubectl auth can-i list secrets --as=<USER> -n <NAMESPACE>`
- Use RBAC to restrict access to specific secrets only

---

## 7. Auditing Secret Access

```bash
# If audit logging is enabled on the cluster
# Events are in the audit log — ask your cluster admin for access

# Check which service accounts have access to a secret
kubectl get rolebindings,clusterrolebindings -A -o json | \
  jq '.items[] | select(.subjects[]?.name == "<SERVICE_ACCOUNT>")'
```

---

## Related

- [kubeconfig management](kubeconfig.md)
- [HashiCorp Vault dynamic secrets](../vault/dynamic-secrets.md)
