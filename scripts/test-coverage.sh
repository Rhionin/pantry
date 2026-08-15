#!/bin/bash
set -e

# Minimum required line coverage percentage
COVERAGE_THRESHOLD=25.3

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo "Running tests with coverage..."
echo "Coverage threshold: ${COVERAGE_THRESHOLD}%"
echo ""

# Run tests with coverage across all packages
go test -cover -coverpkg=./... -coverprofile=coverage.out ./...

if [ $? -ne 0 ]; then
    echo -e "${RED}Tests failed${NC}"
    exit 1
fi

# Calculate total coverage
COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')

echo ""
echo "================================"
echo "Total coverage: ${COVERAGE}%"
echo "Required:       ${COVERAGE_THRESHOLD}%"
echo "================================"

# Compare coverage to threshold
if (( $(echo "$COVERAGE < $COVERAGE_THRESHOLD" | bc -l) )); then
    echo -e "${RED}FAIL: Coverage ${COVERAGE}% is below threshold ${COVERAGE_THRESHOLD}%${NC}"
    exit 1
else
    echo -e "${GREEN}PASS: Coverage ${COVERAGE}% meets or exceeds threshold${NC}"
    
    # If coverage has improved, update the threshold in this script
    if (( $(echo "$COVERAGE > $COVERAGE_THRESHOLD" | bc -l) )); then
        echo ""
        echo -e "${CYAN}Coverage increased! Updating threshold from ${COVERAGE_THRESHOLD}% to ${COVERAGE}%${NC}"
        
        # Get the directory where this script is located
        SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
        SCRIPT_PATH="${SCRIPT_DIR}/test-coverage.sh"
        
        # Update the threshold in the script file
        sed -i.bak "s/^COVERAGE_THRESHOLD=.*/COVERAGE_THRESHOLD=${COVERAGE}/" "$SCRIPT_PATH"
        rm -f "${SCRIPT_PATH}.bak"
        
        echo -e "${GREEN}Threshold updated in ${SCRIPT_PATH}${NC}"
        echo -e "${YELLOW}Remember to commit this change to lock in the new coverage baseline${NC}"
    fi
fi

# Clean up
rm -f coverage.out
