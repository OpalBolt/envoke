#!/usr/bin/env bash
# pre-commit-hook.sh — Git pre-commit hook to detect secrets
#
# Install:
#   cp snippets/pre-commit-hook.sh .git/hooks/pre-commit
#   chmod +x .git/hooks/pre-commit
#
# Or use with the pre-commit framework (recommended):
#   See guides/general/git-security.md

set -euo pipefail

# ---------------------------------------------------------------------------
# Colour helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

FOUND_SECRETS=0

# ---------------------------------------------------------------------------
# 1. Run gitleaks if installed (preferred — comprehensive, maintained)
# ---------------------------------------------------------------------------
if command -v gitleaks >/dev/null 2>&1; then
  echo "🔍 Running gitleaks..."
  if ! gitleaks protect --staged --no-git-semver-check 2>&1; then
    echo -e "${RED}❌ gitleaks detected potential secrets in staged files.${NC}"
    echo "   Review the output above and remove any secrets before committing."
    FOUND_SECRETS=1
  fi
fi

# ---------------------------------------------------------------------------
# 2. Pattern-based fallback scan on staged files
# ---------------------------------------------------------------------------
# Collect staged filenames NUL-delimited to handle spaces/special chars safely
mapfile -d '' STAGED_FILES < <(git diff --cached --name-only --diff-filter=ACM -z 2>/dev/null || true)

if [[ ${#STAGED_FILES[@]} -eq 0 ]]; then
  exit 0
fi

# Patterns that commonly indicate secrets
PATTERNS=(
  'AKIA[0-9A-Z]{16}'                          # AWS Access Key ID
  'sk_live_[0-9a-zA-Z]{24,}'                  # Stripe live key
  'sk_test_[0-9a-zA-Z]{24,}'                  # Stripe test key
  'ghp_[0-9a-zA-Z]{36}'                       # GitHub personal access token
  'glpat-[0-9a-zA-Z_-]{20}'                   # GitLab personal access token
  'xox[baprs]-[0-9a-zA-Z-]+'                  # Slack token
  'eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}' # JWT
  'BEGIN (RSA|EC|OPENSSH|DSA) PRIVATE KEY'     # Private keys
  'password\s*=\s*["\047][^\047"]{4,}'        # Hardcoded password assignments
  'secret\s*=\s*["\047][^\047"]{4,}'          # Hardcoded secret assignments
  'api[_-]?key\s*=\s*["\047][^\047"]{4,}'     # Hardcoded API key assignments
)

echo "🔍 Scanning staged files for secret patterns..."

for FILE in "${STAGED_FILES[@]}"; do
  # Skip binary files
  if git show ":$FILE" | file - | grep -qi binary 2>/dev/null; then
    continue
  fi

  # Skip known safe files
  case "$FILE" in
    *.md|*.txt|*.lock|go.sum|package-lock.json)
      continue
      ;;
  esac

  CONTENT=$(git show ":$FILE" 2>/dev/null || true)

  for PATTERN in "${PATTERNS[@]}"; do
    if echo "$CONTENT" | grep -qiE "$PATTERN" 2>/dev/null; then
      echo -e "${YELLOW}⚠️  Potential secret pattern found in: ${FILE}${NC}"
      echo "   Pattern: $PATTERN"
      FOUND_SECRETS=1
    fi
  done
done

# ---------------------------------------------------------------------------
# 3. Check for .env files being committed
# ---------------------------------------------------------------------------
for FILE in "${STAGED_FILES[@]}"; do
  case "$FILE" in
    .env|.env.*|*.env)
      # Allow template/example files
      case "$FILE" in
        *.example|*.template|*.sample)
          ;;
        *)
          echo -e "${YELLOW}⚠️  .env file staged for commit: ${FILE}${NC}"
          echo "   Add it to .gitignore unless it is an example/template file."
          FOUND_SECRETS=1
          ;;
      esac
      ;;
  esac
done

# ---------------------------------------------------------------------------
# Result
# ---------------------------------------------------------------------------
if [[ $FOUND_SECRETS -eq 1 ]]; then
  echo ""
  echo -e "${RED}🛑 Commit blocked — potential secrets detected.${NC}"
  echo ""
  echo "   To bypass (if this is a false positive):"
  echo "   git commit --no-verify"
  echo ""
  echo "   To fix:"
  echo "   1. Remove the secret from the file"
  echo "   2. Add the file to .gitignore if appropriate"
  echo "   3. Use git add -p to stage only non-secret hunks"
  exit 1
fi

echo -e "${GREEN}✅ No secrets detected in staged files.${NC}"
exit 0
