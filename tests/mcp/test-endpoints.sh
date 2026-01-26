#!/bin/bash
# Orpheus MCP Endpoint Test Script
# Tests MCP protocol compliance: initialize, tools/list, tools/call

set -euo pipefail

HOST="${ORPHEUS_HOST:-http://localhost:7777}"
AGENT="${1:-calculator-python}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

echo ""
echo "========================================="
echo "  Orpheus MCP Endpoint Test"
echo "========================================="
echo "  Host:  $HOST"
echo "  Agent: $AGENT"
echo "========================================="
echo ""

# Test 1: Initialize
echo -e "${YELLOW}Test 1: Initialize${NC}"
INIT_RESPONSE=$(curl -s -i -X POST "$HOST/mcp/agents/$AGENT" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test-client", "version": "1.0.0"}}}')

# Extract session ID from headers
SESSION_ID=$(echo "$INIT_RESPONSE" | grep -i "Mcp-Session-Id:" | awk '{print $2}' | tr -d '\r')

if [[ -z "$SESSION_ID" ]]; then
  echo -e "${RED}FAIL: No session ID returned${NC}"
  exit 1
fi

echo -e "${GREEN}PASS: Session ID: $SESSION_ID${NC}"
echo ""

# Test 2: Tools List
echo -e "${YELLOW}Test 2: tools/list${NC}"
TOOLS_RESPONSE=$(curl -s -X POST "$HOST/mcp/agents/$AGENT" \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}')

# Check for "execute" tool
if echo "$TOOLS_RESPONSE" | grep -q '"name":"execute"'; then
  echo -e "${GREEN}PASS: 'execute' tool found${NC}"
else
  echo -e "${RED}FAIL: 'execute' tool not found${NC}"
  echo "Response: $TOOLS_RESPONSE"
  exit 1
fi
echo ""

# Test 3: Tool Execution
echo -e "${YELLOW}Test 3: tools/call${NC}"
EXEC_RESPONSE=$(curl -s -X POST "$HOST/mcp/agents/$AGENT" \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d '{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "execute", "arguments": {"input": {"test": true}}}}')

# Check for result (even if agent errors, MCP should return result)
if echo "$EXEC_RESPONSE" | grep -q '"result"'; then
  echo -e "${GREEN}PASS: Tool execution returned result${NC}"
else
  echo -e "${RED}FAIL: No result in response${NC}"
  echo "Response: $EXEC_RESPONSE"
  exit 1
fi
echo ""

# Summary
echo "========================================="
echo -e "${GREEN}All MCP tests passed!${NC}"
echo "========================================="
echo ""
echo "MCP endpoint: $HOST/mcp/agents/$AGENT"
echo ""
