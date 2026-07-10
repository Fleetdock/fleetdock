#!/usr/bin/env bash
# Apply the main-branch ruleset defined in .github/rulesets/protect-main.json
#
# Requirements (one of):
#   - Public repository, OR
#   - GitHub Pro / Team / Enterprise (private repos need a paid plan for rulesets)
#
# Usage:
#   ./scripts/setup-branch-protection.sh
#   ./scripts/setup-branch-protection.sh --dry-run
#
# You need `gh` authenticated with admin access to the repository.
set -euo pipefail

REPO="${GITHUB_REPOSITORY:-TajBrains/db-manager}"
RULESET_FILE="$(cd "$(dirname "$0")/.." && pwd)/.github/rulesets/protect-main.json"
DRY_RUN=false

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    -h|--help)
      sed -n '2,14p' "$0"
      exit 0
      ;;
    *) echo "unknown argument: $arg" >&2; exit 1 ;;
  esac
done

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI is required" >&2
  exit 1
fi

if [[ ! -f "$RULESET_FILE" ]]; then
  echo "error: ruleset file not found: $RULESET_FILE" >&2
  exit 1
fi

visibility="$(gh api "repos/${REPO}" --jq .visibility 2>/dev/null || echo unknown)"
echo "Repository: ${REPO} (visibility: ${visibility})"

if [[ "$DRY_RUN" == true ]]; then
  echo "Dry run — would POST ruleset:"
  cat "$RULESET_FILE"
  exit 0
fi

# Remove an existing ruleset with the same name so the script is idempotent.
existing_id=""
if rulesets_json="$(gh api "repos/${REPO}/rulesets" 2>/dev/null)"; then
  existing_id="$(echo "$rulesets_json" | jq -r '.[] | select(.name=="Protect main") | .id' 2>/dev/null | head -1 || true)"
fi
if [[ -n "$existing_id" && "$existing_id" != "null" ]]; then
  echo "Deleting existing ruleset id=${existing_id}…"
  gh api -X DELETE "repos/${REPO}/rulesets/${existing_id}" >/dev/null
fi

echo "Creating ruleset from ${RULESET_FILE}…"
if gh api -X POST "repos/${REPO}/rulesets" --input "$RULESET_FILE" 2>/tmp/ruleset-err.log; then
  echo "Done. Verify at: https://github.com/${REPO}/settings/rules"
  exit 0
fi

echo "" >&2
echo "Failed to create ruleset via API." >&2
if [[ -f /tmp/ruleset-err.log ]]; then
  cat /tmp/ruleset-err.log >&2
fi
echo "" >&2
echo "Common causes:" >&2
echo "  • Private repo on GitHub Free — upgrade to Pro or make the repository public." >&2
echo "  • Token lacks admin:repo_hook / admin:org permissions." >&2
echo "" >&2
echo "Manual setup (GitHub UI):" >&2
echo "  1. Repo → Settings → Rules → Rulesets → New ruleset → Import a ruleset" >&2
echo "  2. Upload: .github/rulesets/protect-main.json" >&2
echo "  Or see docs/BRANCH_PROTECTION.md" >&2
exit 1
