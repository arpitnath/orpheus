#!/bin/bash
# Load test script for calculator-agent-js
# Tests autoscaler behavior with concurrent requests

set -euo pipefail

BASE_URL="http://localhost:7777"
AGENT="calculator-agent-js"
CONCURRENCY=${1:-5}       # Number of concurrent requests
TOTAL_REQUESTS=${2:-20}   # Total requests to send

echo "=========================================="
echo "Orpheus Load Test: $AGENT"
echo "=========================================="
echo "Concurrency: $CONCURRENCY"
echo "Total Requests: $TOTAL_REQUESTS"
echo "=========================================="
echo ""

# Show initial stats
echo "Initial Pool Stats:"
orpheus stats 2>/dev/null | grep -A5 "calculator"
echo ""

# Start time
START_TIME=$(date +%s)

echo "Starting load test..."
echo ""

# Array of sample queries
declare -a QUERIES=(
    "What is 15 times 7?"
    "Calculate 100 divided by 4"
    "Add 250 and 175"
    "What is 89 minus 34?"
    "Multiply 12 by 8"
)

# Send requests with controlled concurrency
for i in $(seq 1 $TOTAL_REQUESTS); do
    # Pick a random query
    QUERY="${QUERIES[$((i % ${#QUERIES[@]}))]}"

    # Launch in background if we haven't hit concurrency limit
    (
        start=$(date +%s%3N 2>/dev/null || date +%s)000

        response=$(curl -s -X POST "${BASE_URL}/v1/agents/${AGENT}/run" \
            -H "Content-Type: application/json" \
            -d "{\"input\": {\"query\": \"$QUERY\"}}" 2>&1)

        end=$(date +%s%3N 2>/dev/null || date +%s)000
        duration=$((end - start))

        if echo "$response" | grep -q '"status":"success"'; then
            echo "[OK] Request $i: ${duration}ms"
        else
            echo "[FAIL] Request $i: ${duration}ms - $response"
        fi
    ) &

    # Control concurrency
    if (( i % CONCURRENCY == 0 )); then
        wait
        echo "--- Batch $((i / CONCURRENCY)) complete ---"
        # Show intermediate stats
        orpheus stats 2>/dev/null | grep -E "calculator|deep-research" | head -2
    fi
done

# Wait for remaining jobs
wait

END_TIME=$(date +%s)
TOTAL_TIME=$((END_TIME - START_TIME))

echo ""
echo "=========================================="
echo "Load Test Complete"
echo "=========================================="
echo "Total Time: ${TOTAL_TIME}s"
echo "Requests: $TOTAL_REQUESTS"
if [ "$TOTAL_TIME" -gt 0 ]; then
    echo "Throughput: $(echo "scale=2; $TOTAL_REQUESTS / $TOTAL_TIME" | bc 2>/dev/null || echo 'N/A') req/s"
fi
echo ""

# Show final stats
echo "Final Pool Stats:"
orpheus stats 2>/dev/null | grep -A10 "Per-Agent"
