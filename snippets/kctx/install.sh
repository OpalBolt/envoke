#!/usr/bin/env bash
# install.sh — Install kctx.sh to ~/.config/kctx/
#
# Usage:
#   ./snippets/kctx/install.sh          # install or upgrade
#   ./snippets/kctx/install.sh --check  # check if installed and current

set -euo pipefail

INSTALL_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/kctx"
SCRIPT_NAME="kctx.sh"
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
    echo "✅ kctx is already up to date at $SCRIPT_DST"
    exit 0
  fi

  if $check_only; then
    echo "⚠️  kctx is outdated at $SCRIPT_DST"
    echo "   Run: ./snippets/kctx/install.sh  to update"
    exit 1
  fi

  echo "🔄 Updating kctx.sh..."
else
  if $check_only; then
    echo "⚠️  kctx is not installed"
    echo "   Run: ./snippets/kctx/install.sh  to install"
    exit 1
  fi

  echo "📦 Installing kctx.sh..."
fi

mkdir -p "$INSTALL_DIR"
cp "$SCRIPT_SRC" "$SCRIPT_DST"
chmod 755 "$SCRIPT_DST"

echo "✅ Installed to $SCRIPT_DST"
echo ""
echo "── Add to ~/.bashrc or ~/.zshrc ─────────────────────────────────────────"
echo ""
echo "  source \"$SCRIPT_DST\""
echo ""
echo "── Usage ────────────────────────────────────────────────────────────────"
echo ""
echo "  kctx prod                        # vault secret/k8s/prod"
echo "  kctx prod bw://k8s-prod          # Bitwarden item (kubeconfig field)"
echo "  kctx_status                      # show current context"
echo "  kctx_clear                       # unset KUBECONFIG"
echo "  kctx_cache_clear                 # flush cached kubeconfigs"
echo "─────────────────────────────────────────────────────────────────────────"
