#!/usr/bin/env bash
set -euo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Output file path
REPORT_FILE=".reports/codemap-diff.txt"

echo -e "${BLUE}=== Codemap Update Context ===${NC}\n"

# Step 1: Extract last processed SHA
echo -e "${YELLOW}[1/5] Checking last processed commit...${NC}"
LAST_SHA=$(grep '^last-sha:' "$REPORT_FILE" 2>/dev/null | awk '{print $2}' || echo "")

if [ -z "$LAST_SHA" ]; then
    echo -e "${YELLOW}No previous run found. Will perform full scan.${NC}"
    BASE_SHA=""
else
    echo -e "Last processed: ${GREEN}$LAST_SHA${NC}"
    BASE_SHA="$LAST_SHA"
fi

# Step 2: Get current HEAD
echo -e "\n${YELLOW}[2/5] Getting current HEAD...${NC}"
HEAD_SHA=$(git rev-parse HEAD)
HEAD_SHORT=$(git rev-parse --short HEAD)
echo -e "Current HEAD: ${GREEN}$HEAD_SHA${NC} ($HEAD_SHORT)"

# Step 3: Check for new commits
echo -e "\n${YELLOW}[3/5] Checking for new commits...${NC}"
if [ -n "$BASE_SHA" ]; then
    NEW_COMMITS=$(git log "$BASE_SHA..HEAD" --oneline 2>/dev/null || echo "")
    if [ -z "$NEW_COMMITS" ]; then
        echo -e "${GREEN}✓ Codemaps are up to date.${NC}"
        exit 0
    fi
    COMMIT_RANGE="${BASE_SHA:0:7}..${HEAD_SHORT}"
else
    NEW_COMMITS=$(git log --oneline -n 10)
    COMMIT_RANGE="initial..${HEAD_SHORT}"
fi

COMMIT_COUNT=$(echo "$NEW_COMMITS" | wc -l | tr -d ' ')
echo -e "Found ${GREEN}$COMMIT_COUNT${NC} new commit(s)"

# Step 4: Collect diff
echo -e "\n${YELLOW}[4/5] Collecting changes...${NC}"
if [ -n "$BASE_SHA" ]; then
    DIFF_NAMES=$(git diff "$BASE_SHA..HEAD" --name-status)
else
    # For initial run, list all tracked files
    DIFF_NAMES=$(git ls-tree -r HEAD --name-status)
fi

FILE_COUNT=$(echo "$DIFF_NAMES" | grep -v '^$' | wc -l | tr -d ' ')
echo -e "Changed files: ${GREEN}$FILE_COUNT${NC}"

# Step 5: Output structured report
echo -e "\n${YELLOW}[5/5] Generating report...${NC}"

echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}METADATA${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo "BASE_SHA: ${BASE_SHA:-<empty>}"
echo "HEAD_SHA: $HEAD_SHA"
echo "HEAD_SHORT: $HEAD_SHORT"
echo "COMMIT_RANGE: $COMMIT_RANGE"
echo "COMMIT_COUNT: $COMMIT_COUNT"
echo "FILE_COUNT: $FILE_COUNT"
echo "DATE: $(date +%Y-%m-%d)"

echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}COMMITS${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo "$NEW_COMMITS"

echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}CHANGED FILES${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo "$DIFF_NAMES"

# Show file breakdown by category
echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}CHANGE BREAKDOWN${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

ADDED=$(echo "$DIFF_NAMES" | grep -c '^A' || echo "0")
MODIFIED=$(echo "$DIFF_NAMES" | grep -c '^M' || echo "0")
DELETED=$(echo "$DIFF_NAMES" | grep -c '^D' || echo "0")
RENAMED=$(echo "$DIFF_NAMES" | grep -c '^R' || echo "0")

echo "Added:    $ADDED"
echo "Modified: $MODIFIED"
echo "Deleted:  $DELETED"
echo "Renamed:  $RENAMED"

# Optional: Show diff statistics
if [ -n "$BASE_SHA" ]; then
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}DIFF STATS${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    git diff "$BASE_SHA..HEAD" --stat
fi

echo -e "\n${GREEN}✓ Context collection complete${NC}"
echo -e "${YELLOW}Next: Analyze changes and update relevant codemaps${NC}"
