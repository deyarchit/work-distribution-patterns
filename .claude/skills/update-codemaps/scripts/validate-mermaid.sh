#!/usr/bin/env bash
set -euo pipefail

# validate-mermaid.sh — Validate all Mermaid blocks in docs/CODEMAPS/*.md
# Requires mmdc (mermaid-cli). If not available, warns and exits cleanly.
# Usage: bash ./scripts/validate-mermaid.sh

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

CODEMAPS_DIR="docs/CODEMAPS"
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

echo -e "${BLUE}=== Mermaid Diagram Validation ===${NC}\n"

# Resolve mmdc command
MMDC_CMD=""
if command -v mmdc &>/dev/null; then
    MMDC_CMD="mmdc"
elif command -v npx &>/dev/null; then
    echo -e "${YELLOW}mmdc not found globally — using npx (slow on first run)${NC}\n"
    MMDC_CMD="npx --yes -p @mermaid-js/mermaid-cli mmdc"
else
    echo -e "${YELLOW}⚠ Neither mmdc nor npx found — skipping validation${NC}"
    echo -e "${YELLOW}  To enable: npm install -g @mermaid-js/mermaid-cli${NC}"
    exit 0
fi

if [ ! -d "$CODEMAPS_DIR" ]; then
    echo -e "${YELLOW}No directory at $CODEMAPS_DIR — nothing to validate${NC}"
    exit 0
fi

ERRORS=0
TOTAL=0

for FILE in "$CODEMAPS_DIR"/*.md; do
    [ -f "$FILE" ] || continue

    # Skip files with no mermaid blocks
    if ! grep -q '```mermaid' "$FILE"; then
        continue
    fi

    TOTAL=$((TOTAL + 1))
    OUT="$WORK_DIR/$(basename "$FILE" .md).svg"

    if $MMDC_CMD -i "$FILE" -o "$OUT" 2>/dev/null; then
        echo -e "  ${GREEN}✓${NC} $(basename "$FILE")"
    else
        echo -e "  ${RED}✗${NC} $(basename "$FILE")"
        # Re-run without suppressing stderr so the error is visible
        $MMDC_CMD -i "$FILE" -o "$OUT" 2>&1 | sed 's/^/    /' || true
        echo ""
        ERRORS=$((ERRORS + 1))
    fi
done

echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo "Files with diagrams: $TOTAL"

if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}✗ $ERRORS file(s) contain invalid diagrams — fix before calling finalize.sh${NC}"
    exit 1
fi

if [ $TOTAL -eq 0 ]; then
    echo -e "${YELLOW}No Mermaid diagrams found in $CODEMAPS_DIR${NC}"
    exit 0
fi

echo -e "${GREEN}✓ All diagrams valid${NC}"
