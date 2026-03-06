#!/usr/bin/env bash
set -euo pipefail

# validate-budget.sh — Check that docs/CODEMAPS/*.md and docs/CODEMAPS/details/*.md
# files stay within character budget.
# Character count is a practical proxy for token count (≈ 1 token per 4 chars).
# Usage: bash ./scripts/validate-budget.sh

CODEMAPS_DIR="docs/CODEMAPS"
LIMIT=4000

if [ ! -d "$CODEMAPS_DIR" ]; then
    echo "No directory at $CODEMAPS_DIR — nothing to validate"
    exit 0
fi

OVER=0
TOTAL=0

check_file() {
    local FILE="$1"
    local LABEL="$2"
    TOTAL=$((TOTAL + 1))

    SIZE=$(wc -c < "$FILE")
    PCT=$(( SIZE * 100 / LIMIT ))

    if [ "$SIZE" -gt "$LIMIT" ]; then
        echo "  ✗ $LABEL — $SIZE / $LIMIT chars (${PCT}%, $(( SIZE - LIMIT )) over budget)"
        OVER=$((OVER + 1))
    elif [ "$PCT" -ge 80 ]; then
        echo "  ~ $LABEL — $SIZE / $LIMIT chars (${PCT}%, approaching limit)"
    else
        echo "  ✓ $LABEL — $SIZE / $LIMIT chars (${PCT}%)"
    fi
}

echo "Top-level codemaps:"
for FILE in "$CODEMAPS_DIR"/*.md; do
    [ -f "$FILE" ] || continue
    check_file "$FILE" "$(basename "$FILE")"
done

if [ -d "$CODEMAPS_DIR/details" ]; then
    echo ""
    echo "Detail files:"
    for FILE in "$CODEMAPS_DIR/details"/*.md; do
        [ -f "$FILE" ] || continue
        check_file "$FILE" "details/$(basename "$FILE")"
    done
fi

echo ""
echo "Files checked: $TOTAL"

if [ "$OVER" -gt 0 ]; then
    echo "✗ $OVER file(s) over budget — compress or split before calling finalize.sh"
    exit 1
fi

echo "✓ All codemaps within budget"
