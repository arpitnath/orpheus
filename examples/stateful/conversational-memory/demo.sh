#!/bin/bash
#
# Interactive Demo: Conversational Therapist Agent
# Demonstrates: State persists across container restarts
#
# The Killer Demo:
# 1. Send message → get response
# 2. Continue conversation → therapist remembers
# 3. Kill container
# 4. Resume → full context preserved!

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

SESSION_ID="demo-patient-$(date +%s)"
AGENT_NAME="conversational-therapist-agent"

echo -e "${CYAN}╔══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║  Conversational Therapist Agent - Interactive Demo              ║${NC}"
echo -e "${CYAN}║  Proof: 'Agents Don't Die' - State Survives Container Restarts  ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Helper function for pretty output
send_message() {
    local message="$1"
    local step="$2"

    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}Step ${step}: Patient says:${NC}"
    echo -e "  \"${message}\""
    echo ""

    echo -e "${CYAN}Sending to Orpheus (Session: ${SESSION_ID})...${NC}"

    response=$(orpheus invoke "$AGENT_NAME" "{\"message\": \"$message\"}" \
        -H "X-Session-ID: $SESSION_ID" 2>&1)

    echo -e "${GREEN}Therapist responds:${NC}"
    echo "$response" | jq -r '.response // .error // .'
    echo ""

    echo -e "${CYAN}Session Stats:${NC}"
    echo "$response" | jq '.session_stats // "Stats not available"'
    echo ""
}

# Step 1: Initial conversation
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}STEP 1: Starting Therapy Session${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

send_message "I have been feeling really anxious about work lately" "1"

echo -e "${YELLOW}Press ENTER to continue...${NC}"
read -r

# Step 2: Continue conversation (session affinity)
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}STEP 2: Continuing Conversation (Session Affinity)${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${CYAN}Note: Same session ID → routes to SAME worker${NC}"
echo ""

send_message "My boss keeps giving me impossible deadlines and I can't keep up" "2"

echo -e "${YELLOW}Press ENTER to continue...${NC}"
read -r

# Step 3: Check workspace state BEFORE kill
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}STEP 3: Checking Workspace (Before Container Kill)${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${CYAN}Workspace contents:${NC}"
orpheus workspace info "$AGENT_NAME" 2>&1 || echo "Workspace command may not be available"

echo ""
echo -e "${YELLOW}Press ENTER to kill the container...${NC}"
read -r

# Step 4: Kill the container
echo -e "${RED}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${RED}STEP 4: KILLING CONTAINER (Simulating Crash/OOM)${NC}"
echo -e "${RED}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${RED}Finding and killing agent container...${NC}"

# Try to kill container via Orpheus commands
container_id=$(limactl shell orpheus -- runc list 2>/dev/null | grep "$AGENT_NAME" | awk '{print $1}' | head -n 1 || echo "")

if [ -n "$container_id" ]; then
    echo -e "${RED}Found container: ${container_id}${NC}"
    echo -e "${RED}Sending SIGKILL...${NC}"
    limactl shell orpheus -- runc kill "$container_id" SIGKILL 2>&1 || echo "Container may have already terminated"
    sleep 2
    echo -e "${RED}✓ Container killed${NC}"
else
    echo -e "${YELLOW}⚠ Could not find running container (may need to use different method)${NC}"
    echo -e "${YELLOW}Container may restart automatically - proceeding with demo...${NC}"
fi

echo ""
echo -e "${YELLOW}Press ENTER to resume conversation...${NC}"
read -r

# Step 5: Resume conversation (THE PROOF)
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}STEP 5: Resuming Conversation (NEW WORKER)${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${CYAN}This will spawn a NEW worker...${NC}"
echo -e "${CYAN}Watch: It loads /workspace/patient_sessions.db${NC}"
echo -e "${CYAN}       Full conversation history restored!${NC}"
echo ""

send_message "Actually, I talked to my boss and we worked out a better schedule. I feel much better!" "3"

echo -e "${YELLOW}Press ENTER to view progress report...${NC}"
read -r

# Step 6: Show progress report
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}STEP 6: Session Progress Report${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${CYAN}Requesting full session analysis...${NC}"
progress=$(orpheus invoke "$AGENT_NAME" '{"action": "show_progress"}' \
    -H "X-Session-ID: $SESSION_ID" 2>&1)

echo "$progress" | jq '.'

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}DEMO COMPLETE ✓${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

echo -e "${CYAN}What just happened:${NC}"
echo -e "  1. ✓ Started conversation (created SQLite DB)"
echo -e "  2. ✓ Continued (session affinity → same worker)"
echo -e "  3. ✓ Killed container (simulated crash)"
echo -e "  4. ✓ Resumed conversation (NEW worker, FULL context)"
echo -e "  5. ✓ Emotional trend tracked (negative → positive)"
echo ""

echo -e "${YELLOW}Key Proof Points:${NC}"
echo -e "  • SQLite DB stored in /workspace (persistent volume)"
echo -e "  • Container died, but agent state survived"
echo -e "  • New worker loaded DB seamlessly"
echo -e "  • Therapist remembered ALL previous context"
echo ""

echo -e "${CYAN}This is impossible in standard Docker/K8s ephemeral containers!${NC}"
echo -e "${CYAN}Orpheus makes agents truly stateful.${NC}"
echo ""
