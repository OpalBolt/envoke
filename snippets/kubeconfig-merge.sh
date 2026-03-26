#!/usr/bin/env bash
# kubeconfig-merge.sh — Safely merge kubeconfig files
#
# Usage:
#   ./kubeconfig-merge.sh <new-kubeconfig-file>
#   ./kubeconfig-merge.sh <new-kubeconfig-file> --output <output-file>
#
# By default, merges into ~/.kube/config (with a backup).
# Use --output to write to a specific file instead.

set -euo pipefail

usage() {
  cat >&2 << 'EOF'
Usage:
  kubeconfig-merge.sh <new-kubeconfig-file> [--output <output-file>]

Options:
  --output <file>   Write merged config to <file> instead of ~/.kube/config
  --dry-run         Print the merged config without writing

Examples:
  kubeconfig-merge.sh ~/Downloads/client-a.kubeconfig
  kubeconfig-merge.sh ~/Downloads/client-a.kubeconfig --output ~/.kube/client-a-merged.config
  kubeconfig-merge.sh ~/Downloads/cluster.kubeconfig --dry-run
EOF
  exit 1
}

[[ $# -lt 1 ]] && usage

NEW_KUBECONFIG="$1"
shift

OUTPUT=""
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)
      OUTPUT="${2:?--output requires a file path}"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    *)
      echo "❌ Unknown option: $1" >&2
      usage
      ;;
  esac
done

# Resolve output target
OUTPUT="${OUTPUT:-${HOME}/.kube/config}"
EXISTING="${HOME}/.kube/config"

# Validate input
if [[ ! -f "$NEW_KUBECONFIG" ]]; then
  echo "❌ File not found: $NEW_KUBECONFIG" >&2
  exit 1
fi

# Verify it looks like a kubeconfig
if ! grep -q 'apiVersion' "$NEW_KUBECONFIG" 2>/dev/null; then
  echo "❌ '$NEW_KUBECONFIG' does not appear to be a valid kubeconfig file" >&2
  exit 1
fi

echo "📋 New kubeconfig contexts:"
KUBECONFIG="$NEW_KUBECONFIG" kubectl config get-contexts 2>/dev/null || \
  grep -E '^\s+name:' "$NEW_KUBECONFIG" | awk '{print "  -", $2}'

# Check for context name collisions
if [[ -f "$EXISTING" ]]; then
  EXISTING_CONTEXTS=$(kubectl config get-contexts -o name --kubeconfig="$EXISTING" 2>/dev/null || true)
  NEW_CONTEXTS=$(kubectl config get-contexts -o name --kubeconfig="$NEW_KUBECONFIG" 2>/dev/null || true)

  while IFS= read -r ctx; do
    if echo "$EXISTING_CONTEXTS" | grep -qxF "$ctx"; then
      echo "⚠️  Context collision: '$ctx' already exists in $OUTPUT"
      echo "   The new definition will overwrite the existing one."
    fi
  done <<< "$NEW_CONTEXTS"
fi

# Perform merge
echo ""
if [[ $DRY_RUN -eq 1 ]]; then
  echo "🔍 Dry-run: merged output would be:"
  if [[ -f "$EXISTING" ]]; then
    KUBECONFIG="${EXISTING}:${NEW_KUBECONFIG}" kubectl config view --flatten
  else
    KUBECONFIG="$NEW_KUBECONFIG" kubectl config view --flatten
  fi
  exit 0
fi

# Backup existing config
if [[ -f "$OUTPUT" ]]; then
  BACKUP="${OUTPUT}.bak.$(date +%Y%m%d_%H%M%S)"
  cp "$OUTPUT" "$BACKUP"
  echo "💾 Backed up existing config to: $BACKUP"
fi

# Ensure .kube directory exists
mkdir -p "$(dirname "$OUTPUT")"

# Merge
if [[ -f "$EXISTING" && "$OUTPUT" == "$EXISTING" ]]; then
  KUBECONFIG="${EXISTING}:${NEW_KUBECONFIG}" kubectl config view --flatten > "${OUTPUT}.tmp"
  mv "${OUTPUT}.tmp" "$OUTPUT"
elif [[ -f "$EXISTING" ]]; then
  KUBECONFIG="${EXISTING}:${NEW_KUBECONFIG}" kubectl config view --flatten > "$OUTPUT"
else
  cp "$NEW_KUBECONFIG" "$OUTPUT"
fi

chmod 600 "$OUTPUT"

echo "✅ Merged kubeconfig written to: $OUTPUT"
echo ""
echo "📋 All contexts in merged config:"
kubectl config get-contexts --kubeconfig="$OUTPUT"
echo ""
echo "💡 To use: export KUBECONFIG=\"$OUTPUT\""
