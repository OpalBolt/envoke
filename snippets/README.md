# Snippets

Snippets are ready-to-source shell functions for common secret-management tasks. Copy them into your project, `source` them in your shell profile, or reference them directly from GitHub using the patterns below.

They are deliberately minimal — single-purpose functions with no external dependencies beyond the tool they wrap (Vault CLI, Bitwarden CLI, git, kubectl).

---

## The One Pattern You Need: `resolve-env-refs.sh`

Everything related to loading secrets into your environment is handled by a single script: [`resolve-env-refs.sh`](resolve-env-refs.sh).

Store references — not secrets — in your `.env` file:

```bash
# .env (safe to commit — contains no secret values)
DATABASE_URL=bw://prod-db/password
DATABASE_USER=bw://prod-db/username
STRIPE_KEY=bw://stripe-api/field:api_key
VAULT_TOKEN=vault://secret/myproject/app#token
```

Then choose how you want to resolve them:

### Pattern 1 — direnv (recommended)

[direnv](https://direnv.net/) automatically loads your env when you enter the directory and **unloads it when you leave** — no cleanup needed.

Since the repo is private, use `gh api` to download and cache the script locally. Pin to a specific commit SHA and verify the hash on each cache miss. Generate the hash:

```bash
openssl dgst -sha256 -binary snippets/resolve-env-refs.sh | base64
```

```bash
# .envrc
_script="${HOME}/.cache/resolve-env-refs-<SHA>.sh"
_expected_hash="sha256-<HASH>"
if [[ ! -f "$_script" ]]; then
  gh api repos/opalbolt/envoke/contents/snippets/resolve-env-refs.sh?ref=<SHA> \
    -H "Accept: application/vnd.github.raw" > "$_script"
  _actual=$(openssl dgst -sha256 -binary "$_script" | base64)
  [[ "sha256-$_actual" == "$_expected_hash" ]] \
    || { rm -f "$_script"; echo "resolve-env-refs: hash mismatch — aborting"; exit 1; }
fi
source "$_script"
source <(resolve_env_file .env)
```

```bash
direnv allow .   # grant permission once per project
```

**Requires:** `gh` CLI authenticated (`gh auth login`). The script is downloaded once and cached; subsequent `direnv` reloads skip the network call.

### Pattern 2 — self-loading .env (standalone shell)

Put the loader as line 1 of your `.env`. Uses `gh api` for private-repo access. When you `source .env`, the script:
1. Fetches the resolver via `gh api` (uses existing `gh auth` — no extra token needed)
2. Detects bash or zsh automatically — no shell-specific syntax needed
3. Unloads any previously active env (prints a message when switching projects)
4. Resolves all `bw://` and `vault://` references and exports the values
5. Returns early so the raw reference strings below never execute as shell assignments
6. Registers `unload_env()` + `trap unload_env EXIT` for cleanup

```bash
# .env
source <(gh api repos/opalbolt/envoke/contents/snippets/resolve-env-refs.sh?ref=<SHA> \
  -H "Accept: application/vnd.github.raw") \
  && declare -f _load_self_env &>/dev/null \
  && _load_self_env \
  && return 0 2>/dev/null; true

# References below are parsed by resolve_env_file above — not executed by bash/zsh:
DATABASE_URL=bw://prod-db/password
STRIPE_KEY=bw://stripe-api/field:api_key
VAULT_TOKEN=vault://secret/myproject/app#token
```

```bash
source .env        # resolves all refs, registers EXIT trap, unloads previous if any
unload_env         # optional: manual cleanup before shell exits
echo $_LOADED_ENV_FILE   # shows which env is currently active
```

**Requires:** `gh` CLI authenticated (`gh auth login`).

### Pattern 3 — exec mode (safest: secrets never enter your shell)

```bash
# Resolved values are injected directly into the child process — never in your shell
./snippets/resolve-env-refs.sh .env -- node server.js
./snippets/resolve-env-refs.sh .env -- python app.py
```

### Reference syntax

| Reference | Retrieves |
|-----------|-----------|
| `bw://item-name` | Bitwarden item password (default) |
| `bw://item-name/password` | Bitwarden password field |
| `bw://item-name/username` | Bitwarden username field |
| `bw://item-name/note` | Bitwarden notes field |
| `bw://item-name/field:fname` | Bitwarden custom field named `fname` |
| `vault://secret/path#field` | Vault KV field at path |
| `vault://secret/path` | All fields from a Vault KV path |

**Prerequisites:**
- **Bitwarden:** `bw` CLI installed. `BW_SESSION` set or vault will prompt interactively.
- **Vault:** `vault` CLI installed. `VAULT_ADDR` and `VAULT_TOKEN` set (see [authentication guide](../guides/vault/authentication.md)).

---

## Other Snippets

### [`pre-commit-hook.sh`](pre-commit-hook.sh)

Git pre-commit hook that scans staged files for secrets before they are committed. Uses `gitleaks` if installed, with a regex fallback for common patterns (AWS keys, GitHub tokens, Stripe keys, JWT, private keys).

**Install as a hook:**

```bash
cp snippets/pre-commit-hook.sh /path/to/your-repo/.git/hooks/pre-commit
chmod +x /path/to/your-repo/.git/hooks/pre-commit
```

**Install globally for all new repos:**

```bash
mkdir -p ~/.git-hooks
cp snippets/pre-commit-hook.sh ~/.git-hooks/pre-commit
chmod +x ~/.git-hooks/pre-commit
git config --global core.hooksPath ~/.git-hooks
```

**Related:** [Git security guide](../guides/general/git-security.md)

---

### [`kctx.sh`](kctx.sh)

Ephemeral kubeconfig switching via Vault. Fetches a kubeconfig into a RAM-backed tmpfile (`/dev/shm` on Linux) and exports `KUBECONFIG` pointing at it. The tmpfile is cleaned up on the next call, on `kctx_clear`, or when the shell exits.

**Functions:**

| Function | Description |
|----------|-------------|
| `kctx <env> [vault-path]` | Fetch kubeconfig from `secret/k8s/<env>` (or explicit path) |
| `kctx_clear` | Remove the tmpfile and unset `KUBECONFIG` |
| `kctx_status` | Show the active `KUBECONFIG` and current context |

```bash
source snippets/kctx.sh  # add to ~/.bashrc or ~/.zshrc

kctx prod                             # fetches from secret/k8s/prod
kctx staging secret/infra/k8s/staging # explicit Vault path
kctx_status                           # show what's active
kctx_clear                            # clean up
```

**Related:** [Kubeconfig guide](../guides/kubernetes/kubeconfig.md)

---

### [`kubeconfig-merge.sh`](kubeconfig-merge.sh)

Safely merge kubeconfig files with conflict detection, backup creation, and dry-run support.

```bash
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig --dry-run
./snippets/kubeconfig-merge.sh ~/.kube/new-cluster.kubeconfig
```

**Related:** [Kubeconfig guide](../guides/kubernetes/kubeconfig.md)
