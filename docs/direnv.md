# direnv integration

[direnv](https://direnv.net/) manages per-directory environments by evaluating an `.envrc` file when you `cd` into a directory. It owns the load/unload lifecycle itself, so envoke integrates with it cleanly — no EXIT trap conflicts, no double-unload.

## How it works

envoke detects direnv automatically. When `DIRENV_DIR` or `DIRENV_FILE` is set in the environment, `envoke resolve` skips emitting its own EXIT trap, since direnv handles cleanup when you leave the directory.

> **Note:** During `.envrc` evaluation, shell functions from `envoke shell-init` are not available, so your direnv helpers should call the `envoke` binary directly. After direnv has loaded the environment, those shell functions are still available in your interactive shell if you added `envoke shell-init` to your shell config.

## Setup

### 1. Add a `use_envoke` helper to direnvrc

Create or edit `~/.config/direnv/direnvrc`:

```bash
use_envoke() {
  local file="${1:-.env}"
  watch_file "$file"
  eval "$(envoke unload 2>/dev/null || true)"
  eval "$(envoke resolve "$file")"
}
```

`watch_file` tells direnv to re-evaluate `.envrc` whenever the `.env` file changes.
The `envoke unload` call ensures stale variables are cleared before loading fresh ones.

### 2. Use it in your project `.envrc`

```bash
# .envrc
use envoke .env
```

Or with an explicit file path:

```bash
use envoke secrets/prod.env
```

### 3. Allow the directory

```bash
direnv allow
```

## Example

**`.env`:**
```bash
DB_HOST=postgres.internal
DB_PASSWORD=bw://database/myapp-prod
KCTX_PROD=bw://kubernetes/prod-cluster
```

**`.envrc`:**
```bash
use envoke .env
```

When you `cd` into the directory, direnv evaluates `.envrc`, which calls `envoke resolve .env`. Secrets are loaded and the `prod` kubeconfig is available via `envoke switch prod`. When you `cd` out, direnv unsets them automatically.

## Notes

- **Do not** use `envoke shell-init` inside `.envrc`. The shell-init snippet is for interactive shell configs (`~/.bashrc`, `~/.zshrc`) only.
- The background watcher (started by shell-init) is not involved in direnv usage — direnv handles cleanup through its own hooks.
- If you use both shell-init in your shell config and direnv, they coexist without conflict: shell-init's EXIT trap and PROMPT_COMMAND hook are in the interactive shell layer; direnv operates at the cd-hook layer.
- Passwords are still prompted interactively when the cache is cold. Each developer will be prompted for their own Bitwarden password.
