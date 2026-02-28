#!/usr/bin/env bash
set -euo pipefail

# finalize.sh — Write codemap-diff.txt after a successful codemap update.
# Call this only when analysis is complete to avoid masking failures.
# Usage: bash ./scripts/finalize.sh

REPORT_FILE=".reports/codemap-diff.txt"

HEAD_SHA=$(git rev-parse HEAD)
HEAD_SHORT=$(git rev-parse --short HEAD)
DATE=$(date +%Y-%m-%d)

# Build commit range using the previous last-sha
BASE_SHA=$(grep '^last-sha:' "$REPORT_FILE" 2>/dev/null | awk '{print $2}' || echo "")
if [ -n "$BASE_SHA" ]; then
    COMMIT_RANGE="${BASE_SHA:0:7}..${HEAD_SHORT}"
else
    COMMIT_RANGE="initial..${HEAD_SHORT}"
fi

mkdir -p "$(dirname "$REPORT_FILE")"
cat > "$REPORT_FILE" <<EOF
last-sha: $HEAD_SHA
updated: $DATE
commit-range: $COMMIT_RANGE
EOF

echo "✓ codemap-diff.txt updated (last-sha: $HEAD_SHORT, range: $COMMIT_RANGE)"
