#!/bin/bash
# Crash Recovery Test - Validates ExecLog crash detection

set -euo pipefail

ORPHEUS_URL="${ORPHEUS_URL:-http://localhost:7777}"
AGENT="${1:-rag-agent}"
NUM_REQUESTS="${2:-15}"

echo "=========================================="
echo "Crash Recovery Test"
echo "=========================================="
echo "Agent:    $AGENT"
echo "Requests: $NUM_REQUESTS"
echo ""

# Send requests in background (long-running RAG queries)
echo "Launching $NUM_REQUESTS concurrent requests..."
for i in $(seq 1 $NUM_REQUESTS); do
    (curl -s -X POST "$ORPHEUS_URL/v1/agents/$AGENT/run" \
        -H "Content-Type: application/json" \
        -d '{"input":{"question":"Explain FAISS vector similarity search in detail"}}' \
        > /dev/null 2>&1) &
done
echo "✓ Requests launched"
echo ""

# Wait briefly for requests to enter STARTED state
echo "Waiting 2 seconds for requests to start executing..."
sleep 2

# Show current state
echo "Checking ExecLog state..."
node dist/index.js execlog list "$AGENT" -s STARTED --limit 20 2>&1 | head -20
node dist/index.js execlog list "$AGENT" -s QUEUED --limit 20 2>&1 | head -20
echo ""

# Kill daemon (simulate crash)
echo "KILLING DAEMON (simulating crash)..."
limactl shell orpheus -- sudo pkill -9 orpheusd 2>/dev/null
echo "✓ Daemon killed"
echo ""

# Wait for background processes
sleep 3
wait 2>/dev/null || true

# Restart daemon
echo "Restarting daemon..."
limactl shell orpheus -- bash -c "sudo nohup /usr/local/bin/orpheusd --tcp-bind :7777 > /var/log/orpheusd.log 2>&1 &"
sleep 5
echo "✓ Daemon restarted"
echo ""

# Verify daemon is responsive
echo "Testing connection..."
if curl -s "$ORPHEUS_URL/health" > /dev/null 2>&1; then
    echo "✓ Daemon responsive"
else
    echo "Waiting for daemon..."
    sleep 3
    curl -s "$ORPHEUS_URL/health" > /dev/null 2>&1 || echo "Warning: Daemon slow to respond"
fi
echo ""

# Check for crashed requests
echo "=========================================="
echo "Querying ExecLog for CRASHED requests..."
echo "=========================================="
echo ""

node dist/index.js execlog crashed 2>&1

echo ""
echo "All ExecLog states for $AGENT:"
node dist/index.js execlog list "$AGENT" --limit 30 2>&1
echo ""

# Check daemon logs for crash recovery
echo "=========================================="
echo "Daemon crash recovery logs:"
echo "=========================================="
limactl shell orpheus -- grep -i "crash\|recovery" /var/log/orpheusd.log 2>&1 | tail -20 || echo "(No crash recovery logs - safe-first behavior)"
echo ""

echo "=========================================="
echo "Test Complete"
echo "=========================================="
echo ""
echo "Expected behavior:"
echo "- STARTED requests → marked as CRASHED"
echo "- QUEUED requests → stay QUEUED (can be retried)"
echo "- NO auto-retry (safe-first)"
echo ""
