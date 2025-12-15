#!/bin/bash
# Integration tests for AgentScale

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
RUNTIME="$PROJECT_DIR/bin/agentscale-runtime"

# Counters
PASSED=0
FAILED=0

# Header
echo "=========================================="
echo "AgentScale Integration Tests"
echo "=========================================="
echo ""

# Check if runtime binary exists
if [ ! -f "$RUNTIME" ]; then
    echo -e "${RED}Error: Runtime binary not found at $RUNTIME${NC}"
    echo "Run 'make build-runtime' first"
    exit 1
fi

echo "Runtime: $RUNTIME"
echo ""

# Test 1: Simple agent with valid input
echo -n "Test 1: Simple agent - valid input... "
output=$(echo '{"query": "hello"}' | "$RUNTIME" run --no-isolate "$PROJECT_DIR/examples/simple-agent" 2>&1)
if echo "$output" | grep -q '"status": "success"'; then
    echo -e "${GREEN}PASSED${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAILED${NC}"
    echo "  Output: $output"
    FAILED=$((FAILED + 1))
fi

# Test 2: Simple agent with empty input
echo -n "Test 2: Simple agent - empty input... "
output=$(echo '{}' | "$RUNTIME" run --no-isolate "$PROJECT_DIR/examples/simple-agent" 2>&1)
if echo "$output" | grep -q '"status": "success"'; then
    echo -e "${GREEN}PASSED${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAILED${NC}"
    echo "  Output: $output"
    FAILED=$((FAILED + 1))
fi

# Test 3: Check output content
echo -n "Test 3: Simple agent - output content... "
output=$(echo '{"query": "test123"}' | "$RUNTIME" run --no-isolate "$PROJECT_DIR/examples/simple-agent" 2>&1)
if echo "$output" | grep -q "Processed: test123"; then
    echo -e "${GREEN}PASSED${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAILED${NC}"
    echo "  Output: $output"
    FAILED=$((FAILED + 1))
fi

# Test 4: Invalid agent directory
echo -n "Test 4: Invalid agent directory... "
output=$("$RUNTIME" run /nonexistent/path --no-isolate 2>&1) || true
if echo "$output" | grep -q "error"; then
    echo -e "${GREEN}PASSED${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAILED${NC}"
    echo "  Output: $output"
    FAILED=$((FAILED + 1))
fi

# Test 5: Version command
echo -n "Test 5: Version command... "
output=$("$RUNTIME" version 2>&1)
if echo "$output" | grep -q "agentscale-runtime"; then
    echo -e "${GREEN}PASSED${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAILED${NC}"
    echo "  Output: $output"
    FAILED=$((FAILED + 1))
fi

# Test 6: Help command
echo -n "Test 6: Help command... "
output=$("$RUNTIME" help 2>&1)
if echo "$output" | grep -q "Usage:"; then
    echo -e "${GREEN}PASSED${NC}"
    PASSED=$((PASSED + 1))
else
    echo -e "${RED}FAILED${NC}"
    echo "  Output: $output"
    FAILED=$((FAILED + 1))
fi

# Summary
echo ""
echo "=========================================="
echo -e "Results: ${GREEN}$PASSED passed${NC}, ${RED}$FAILED failed${NC}"
echo "=========================================="

# Exit with failure if any tests failed
if [ $FAILED -gt 0 ]; then
    exit 1
fi

echo ""
echo -e "${GREEN}All tests passed!${NC}"
