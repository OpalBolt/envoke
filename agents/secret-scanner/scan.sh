#!/usr/bin/env bash
# scan.sh — Secret scanner agent
#
# Scans a git repository for leaked secrets using gitleaks and regex patterns.
#
# Usage:
#   ./scan.sh [--staged] [--history] [--format json|text] [--audit]

set -euo pipefail

SCAN_MODE="working-tree"   # working-tree | staged | history
OUTPUT_FORMAT="text"
AUDIT_MODE=0
EXIT_CODE=0

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --staged)     SCAN_MODE="staged"; shift ;;
    --history)    SCAN_MODE="history"; shift ;;
    --format)     OUTPUT_FORMAT="${2:?}"; shift 2 ;;
    --audit)      AUDIT_MODE=1; shift ;;
    *)            echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

RESULTS=()
GITLEAKS_AVAILABLE=0
command -v gitleaks >/dev/null 2>&1 && GITLEAKS_AVAILABLE=1

run_gitleaks() {
  case "$SCAN_MODE" in
    staged)
      gitleaks protect --staged --exit-code 1 --report-format json 2>/dev/null
      ;;
    history)
      gitleaks detect --exit-code 1 --report-format json 2>/dev/null
      ;;
    *)
      gitleaks detect --no-git --source . --exit-code 1 --report-format json 2>/dev/null
      ;;
  esac
}

run_pattern_scan() {
  local patterns=(
    'AKIA[0-9A-Z]{16}'
    'sk_(live|test)_[0-9a-zA-Z]{24,}'
    'ghp_[0-9a-zA-Z]{36}'
    'glpat-[0-9a-zA-Z_-]{20}'
    'xox[baprs]-[0-9a-zA-Z-]+'
    'BEGIN (RSA|EC|OPENSSH|DSA) PRIVATE KEY'
  )

  # History mode: scan actual commit content via git log -p, not working tree
  if [[ "$SCAN_MODE" == "history" ]]; then
    echo "ℹ️  Pattern-based history scan: reviewing commit diffs (gitleaks preferred for this)" >&2
    local log_content
    log_content=$(git log --all -p --no-merges 2>/dev/null || true)
    for pattern in "${patterns[@]}"; do
      local match
      match=$(echo "$log_content" | grep -iE "^\\+.*${pattern}" 2>/dev/null | head -5 || true)
      if [[ -n "$match" ]]; then
        RESULTS+=("{\"source\":\"git-history\",\"pattern\":\"$pattern\",\"match\":$(echo "$match" | head -1 | jq -Rs .)}")
      fi
    done
    return
  fi

  # Staged and working-tree modes: use NUL-delimited filenames to handle spaces
  local -a files=()
  case "$SCAN_MODE" in
    staged)
      mapfile -d '' files < <(git diff --cached --name-only --diff-filter=ACM -z 2>/dev/null || true)
      ;;
    *)
      mapfile -d '' files < <(git ls-files -z 2>/dev/null || true)
      ;;
  esac

  for file in "${files[@]}"; do
    [[ -f "$file" ]] || continue
    for pattern in "${patterns[@]}"; do
      if grep -qiE "$pattern" "$file" 2>/dev/null; then
        local line
        line=$(grep -niE "$pattern" "$file" 2>/dev/null | head -1)
        RESULTS+=("{\"file\":$(printf '%s' "$file" | jq -Rs .),\"pattern\":\"$pattern\",\"match\":$(echo "$line" | jq -Rs .)}")
      fi
    done
  done
}

# Run scanners
if [[ $GITLEAKS_AVAILABLE -eq 1 ]]; then
  echo "🔍 Running gitleaks (${SCAN_MODE} mode)..." >&2
  if ! run_gitleaks; then
    EXIT_CODE=1
  fi
else
  echo "⚠️  gitleaks not found — using pattern matching only" >&2
  echo "   Install: brew install gitleaks" >&2
fi

run_pattern_scan

# Output results
if [[ ${#RESULTS[@]} -gt 0 ]]; then
  EXIT_CODE=1
  if [[ "$OUTPUT_FORMAT" == "json" ]]; then
    printf '[%s]\n' "$(IFS=,; echo "${RESULTS[*]}")"
  else
    echo ""
    echo "🚨 Potential secrets found:"
    for r in "${RESULTS[@]}"; do
      echo "  • $(echo "$r" | jq -r '"File: \(.file)  Pattern: \(.pattern)"' 2>/dev/null || echo "$r")"
    done
  fi
fi

if [[ $EXIT_CODE -eq 0 ]]; then
  echo "✅ No secrets detected." >&2
fi

[[ $AUDIT_MODE -eq 1 ]] && exit 0
exit $EXIT_CODE
