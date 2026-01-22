#!/bin/bash
# Load test for RAG agent - demonstrates queue-depth autoscaling
set -e

ORPHEUS_URL="${ORPHEUS_URL:-http://localhost:7777}"
AGENT="rag-agent"
N="${1:-10}"  # Number of jobs (default 10)

echo "=================================================="
echo "RAG Agent Load Test"
echo "=================================================="
echo "Target: $ORPHEUS_URL"
echo "Agent:  $AGENT"
echo "Jobs:   $N"
echo ""

# Questions to ask (rotate through these)
QUESTIONS=(
    "What is KEDA?"
    "Why does CPU autoscaling fail?"
    "What is queue depth scaling?"
    "How does Prometheus monitoring work?"
    "What is FAISS used for?"
    "Explain RAG architecture"
    "What is Ollama?"
    "How does Redis queue work?"
)

# Function to push a single job (async)
push_job() {
    local job_num=$1
    local question="${QUESTIONS[$((job_num % ${#QUESTIONS[@]}))]}"
    local start=$(date +%s.%N)

    result=$(curl -s -X POST "$ORPHEUS_URL/v1/agents/$AGENT/run" \
        -H "Content-Type: application/json" \
        -d "{\"input\":{\"question\":\"$question\"}}" 2>/dev/null)

    local end=$(date +%s.%N)
    local duration=$(echo "$end - $start" | bc)

    # Extract status from result
    status=$(echo "$result" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)

    echo "[Job $job_num] $status (${duration}s) - $question"
}

# Show initial stats
echo "Initial state:"
orpheus stats 2>/dev/null | grep -A5 "rag-agent" || true
echo ""

# Record start time
START_TIME=$(date +%s.%N)
echo "Pushing $N jobs at $(date '+%H:%M:%S')..."
echo ""

# Push all jobs concurrently
for i in $(seq 1 $N); do
    push_job $i &
done

# Wait for all jobs to complete
wait

# Record end time
END_TIME=$(date +%s.%N)
TOTAL_TIME=$(echo "$END_TIME - $START_TIME" | bc)

echo ""
echo "=================================================="
echo "Results"
echo "=================================================="
echo "Total jobs:    $N"
echo "Total time:    ${TOTAL_TIME}s"
echo "Throughput:    $(echo "scale=2; $N / $TOTAL_TIME" | bc) jobs/s"
echo ""

# Show final stats
echo "Final state:"
orpheus stats 2>/dev/null | grep -A5 "rag-agent" || true
