#!/usr/bin/env bash
# install.sh — Install resolve-env-refs.sh to ~/.config/resolve-env-refs/
#
# Usage:
#   ./snippets/resolve-env-refs/install.sh          # install or upgrade
#   ./snippets/resolve-env-refs/install.sh --check  # check if installed and current

set -euo pipefail

INSTALL_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/resolve-env-refs"
SCRIPT_NAME="resolve-env-refs.sh"
SCRIPT_SRC="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)/$SCRIPT_NAME"
SCRIPT_DST="$INSTALL_DIR/$SCRIPT_NAME"

if [[ ! -f "$SCRIPT_SRC" ]]; then
  echo "❌ Source script not found: $SCRIPT_SRC" >&2
  echo "   Run this script from inside the cloned repository." >&2
  exit 1
fi

check_only=false
if [[ "${1:-}" == "--check" ]]; then
  check_only=true
fi

_sha256() {
  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 -binary "$1" | base64
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    echo "❌ Neither openssl nor sha256sum found — cannot verify hashes" >&2
    return 1
  fi
}

if [[ -f "$SCRIPT_DST" ]]; then
  src_hash=$(_sha256 "$SCRIPT_SRC")
  dst_hash=$(_sha256 "$SCRIPT_DST")

  if [[ "$src_hash" == "$dst_hash" ]]; then
    echo "✅ resolve-env-refs is already up to date at $SCRIPT_DST"
    exit 0
  fi

  if $check_only; then
    echo "⚠️  resolve-env-refs is outdated at $SCRIPT_DST"
    echo "   Run: ./snippets/resolve-env-refs/install.sh  to update"
    exit 1
  fi

  echo "🔄 Updating resolve-env-refs.sh..."
else
  if $check_only; then
    echo "⚠️  resolve-env-refs is not installed"
    echo "   Run: ./snippets/resolve-env-refs/install.sh  to install"
    exit 1
  fi

  echo "📦 Installing resolve-env-refs.sh..."
fi

mkdir -p "$INSTALL_DIR"
cp "$SCRIPT_SRC" "$SCRIPT_DST"
chmod 755 "$SCRIPT_DST"

echo "✅ Installed to $SCRIPT_DST"
echo ""
echo "── .env / .envrc ────────────────────────────────────────────────────────"
echo ""
echo "  direnv (.envrc):"
echo "    source \"$SCRIPT_DST\""
echo "    source <(resolve_env_file .env)"
echo ""
echo "  Self-loading .env (first line):"
echo "    source \"$SCRIPT_DST\" \\"
echo "      && declare -f _load_self_env &>/dev/null \\"
echo "      && _load_self_env \\"
echo "      && return 0 2>/dev/null; true"
echo ""
echo "── YAML ─────────────────────────────────────────────────────────────────"
echo ""
echo "  Stream (pipe to tool):"
echo "    source \"$SCRIPT_DST\""
echo "    resolve_yaml_file values.yaml | helm upgrade myapp . -f -"
echo ""
echo "  Exec (temp file, {} replaced with path):"
echo "    resolve_yaml_exec values.yaml -- kubectl apply -f {}"
echo ""
echo "  See: $INSTALL_DIR/README.md  (copy from repo: snippets/resolve-env-refs/README.md)"
echo "─────────────────────────────────────────────────────────────────────────"
