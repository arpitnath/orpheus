#!/bin/bash
# Simple load test for calculator-agent-js

BASE_URL="http://localhost:7777"
AGENT="calculator-agent-js"
API_KEY="${ORPHEUS_API_KEY:-}"
if [ -z "$API_KEY" ]; then
    echo "Error: ORPHEUS_API_KEY environment variable not set"
    echo "Get a key with: orpheus server list-keys"
    exit 1
fi
CONCURRENCY=${1:-5}
TOTAL=${2:-15}

echo "Load Test: $AGENT (concurrency=$CONCURRENCY, total=$TOTAL)"
echo ""
orpheus stats 2>/dev/null | grep -A2 "calculator"
echo ""

START=$(date +%s)

# Send concurrent requests in batches
for batch in $(seq 1 $((TOTAL / CONCURRENCY))); do
    echo "--- Batch $batch ---"

    # Launch CONCURRENCY requests in parallel
    for i in $(seq 1 $CONCURRENCY); do
        req_num=$(( (batch-1) * CONCURRENCY + i ))
        curl -s -X POST "${BASE_URL}/v1/agents/${AGENT}/run" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${API_KEY}" \
            -d '{"input": {"query": "What is 15 times 7?"}}' \
            -o /dev/null -w "Req $req_num: %{http_code} (%{time_total}s)\n" &
    done
    wait

    # Show pool stats
    echo ""
    orpheus stats 2>/dev/null | grep -E "calculator|WORKERS"
    echo ""
done

END=$(date +%s)
echo "Total time: $((END - START))s"
