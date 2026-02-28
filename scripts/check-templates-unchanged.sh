#!/usr/bin/env sh
# Prevent accidental changes to examples/templates/ (pristine snapshots for make reset-examples).
# Usage:
#   ./scripts/check-templates-unchanged.sh           # fail if uncommitted changes in examples/templates/
#   ./scripts/check-templates-unchanged.sh --pre-commit  # fail if any staged file is under examples/templates/
#   ./scripts/check-templates-unchanged.sh --ci [base]   # fail if branch has commits touching examples/templates/ (default base: origin/main)
# Override: ALLOW_TEMPLATE_CHANGES=1 to skip the check (for intentional edits).

set -e
TEMPLATES_DIR="examples/templates"

if [ -n "${ALLOW_TEMPLATE_CHANGES}" ]; then
  exit 0
fi

cd "$(git rev-parse --show-toplevel)"

if [ "$1" = "--pre-commit" ]; then
  # Staged files: reject if any are under examples/templates/
  changed=$(git diff --cached --name-only -- "${TEMPLATES_DIR}")
  if [ -n "$changed" ]; then
    echo "error: commits that modify ${TEMPLATES_DIR}/ are blocked by default (pristine snapshots for make reset-examples)." >&2
    echo "  Staged files in that directory:" >&2
    echo "$changed" | sed 's/^/    /' >&2
    echo "  To allow changes, run with: ALLOW_TEMPLATE_CHANGES=1 git commit ..." >&2
    exit 1
  fi
  exit 0
fi

if [ "$1" = "--ci" ]; then
  base="${2:-origin/main}"
  if ! git rev-parse -q --verify "$base" >/dev/null 2>&1; then
    echo "warning: base ref $base not found; skipping CI template check" >&2
    exit 0
  fi
  changed=$(git diff --name-only "$base"...HEAD -- "${TEMPLATES_DIR}")
  if [ -n "$changed" ]; then
    echo "error: this branch modifies ${TEMPLATES_DIR}/ (pristine snapshots). Changes:" >&2
    echo "$changed" | sed 's/^/  /' >&2
    echo "  If intentional, merge with ALLOW_TEMPLATE_CHANGES=1 or adjust CI." >&2
    exit 1
  fi
  exit 0
fi

# Default: fail if there are uncommitted changes in examples/templates/
if ! git diff --quiet -- "${TEMPLATES_DIR}" 2>/dev/null || ! git diff --cached --quiet -- "${TEMPLATES_DIR}" 2>/dev/null; then
  echo "error: uncommitted changes in ${TEMPLATES_DIR}/ (pristine snapshots for make reset-examples)." >&2
  echo "  To allow edits and commit: ALLOW_TEMPLATE_CHANGES=1" >&2
  echo "  To discard local changes: git checkout -- ${TEMPLATES_DIR}/" >&2
  exit 1
fi
exit 0
