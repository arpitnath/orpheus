#!/bin/bash
# Orpheus Load Test Script
# Uses 'hey' for HTTP load testing to demonstrate queue-depth scaling
#
# Usage:
#   ./load-test.sh <agent-name> [options]
#
# Examples:
#   ./load-test.sh rag-search                    # 100 requests, 10 concurrent
#   ./load-test.sh rag-search -n 200 -c 50       # 200 requests, 50 concurrent
#   ./load-test.sh calculator -p '{"a":1,"b":2}' # Custom payload

set -euo pipefail

# Default configuration
HOST="${ORPHEUS_HOST:-http://localhost:7777}"
REQUESTS=100
CONCURRENCY=10
PAYLOAD=""
AGENT=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

usage() {
    cat <<EOF
Orpheus Load Test

Usage: $(basename "$0") <agent-name> [options]

Options:
  -n, --requests <num>     Total requests to send (default: 100)
  -c, --concurrent <num>   Concurrent requests (default: 10)
  -p, --payload <json>     JSON payload (default: agent-specific)
  -h, --host <url>         Server URL (default: http://localhost:7777)
  --help                   Show this help

Examples:
  $(basename "$0") rag-search                      # Basic load test
  $(basename "$0") rag-search -n 200 -c 50         # Heavy load
  $(basename "$0") calculator -p '{"a":5,"b":3}'   # Custom payload

EOF
    exit 0
}

# Check if hey is installed
check_hey() {
    if ! command -v hey &> /dev/null; then
        echo -e "${RED}Error: 'hey' is not installed${NC}"
        echo ""
        echo "Install with:"
        echo "  brew install hey     # macOS"
        echo "  go install github.com/rakyll/hey@latest  # Go"
        exit 1
    fi
}

# Get default payload for known agents
get_default_payload() {
    local agent="$1"
    case "$agent" in
        rag-search|rag*)
            echo '{"query": "What is machine learning?"}'
            ;;
        calculator*)
            echo '{"a": 15, "b": 7}'
            ;;
        embedding*|embed*)
            echo '{"text": "This is a test sentence for embedding."}'
            ;;
        conversational*|chat*)
            echo '{"message": "Hello, how are you?"}'
            ;;
        *)
            echo '{"input": "test"}'
            ;;
    esac
}

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -n|--requests)
                REQUESTS="$2"
                shift 2
                ;;
            -c|--concurrent)
                CONCURRENCY="$2"
                shift 2
                ;;
            -p|--payload)
                PAYLOAD="$2"
                shift 2
                ;;
            -h|--host)
                HOST="$2"
                shift 2
                ;;
            --help)
                usage
                ;;
            -*)
                echo -e "${RED}Unknown option: $1${NC}"
                usage
                ;;
            *)
                if [[ -z "$AGENT" ]]; then
                    AGENT="$1"
                fi
                shift
                ;;
        esac
    done

    if [[ -z "$AGENT" ]]; then
        echo -e "${RED}Error: Agent name required${NC}"
        echo ""
        usage
    fi

    # Set default payload if not provided
    if [[ -z "$PAYLOAD" ]]; then
        PAYLOAD=$(get_default_payload "$AGENT")
    fi
}

# Check if agent exists
check_agent() {
    local agent="$1"
    local response

    response=$(curl -s -o /dev/null -w "%{http_code}" "${HOST}/v1/agents/${agent}" 2>/dev/null || echo "000")

    if [[ "$response" != "200" ]]; then
        echo -e "${RED}Error: Agent '$agent' not found (HTTP $response)${NC}"
        echo ""
        echo "Available agents:"
        curl -s "${HOST}/v1/agents" 2>/dev/null | jq -r '.[].name' 2>/dev/null || echo "  (unable to list agents)"
        exit 1
    fi
}

# Main
main() {
    check_hey
    parse_args "$@"

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  Orpheus Load Test${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "  Agent:       ${CYAN}${AGENT}${NC}"
    echo -e "  Requests:    ${CYAN}${REQUESTS}${NC}"
    echo -e "  Concurrency: ${CYAN}${CONCURRENCY}${NC}"
    echo -e "  Host:        ${CYAN}${HOST}${NC}"
    echo -e "  Payload:     ${CYAN}${PAYLOAD}${NC}"
    echo ""

    # Check agent exists
    check_agent "$AGENT"

    # Build endpoint URL
    local endpoint="${HOST}/v1/agents/${AGENT}/run"

    echo -e "${YELLOW}Starting load test...${NC}"
    echo ""

    # Run hey
    hey -n "$REQUESTS" -c "$CONCURRENCY" \
        -m POST \
        -H "Content-Type: application/json" \
        -d "$PAYLOAD" \
        "$endpoint"

    echo ""
    echo -e "${GREEN}Load test complete!${NC}"
}

main "$@"
