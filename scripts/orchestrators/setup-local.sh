#!/bin/bash
# Orpheus Setup - Local Development
# Main orchestrator for setting up Orpheus locally

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SETUP_DIR="$SCRIPT_DIR/../setup"
source "$SCRIPT_DIR/../lib/common.sh"

# =============================================================================
# Banner
# =============================================================================
echo ""
echo -e "${BOLD}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║                                                               ║${NC}"
echo -e "${BOLD}║   ${CYAN}ORPHEUS${NC}${BOLD} - Local Development Setup                         ║${NC}"
echo -e "${BOLD}║                                                               ║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# =============================================================================
# Platform Detection
# =============================================================================
OS=$(detect_os)
ARCH=$(detect_arch)

log_info "Detected platform: ${BOLD}$OS/$ARCH${NC}"
echo ""

if [[ "$OS" == "unknown" ]]; then
    log_error "Unsupported operating system"
    exit 1
fi

# =============================================================================
# Parse Arguments
# =============================================================================
SKIP_OLLAMA=false
SKIP_DAEMON=false
SKIP_CLI=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-ollama)
            SKIP_OLLAMA=true
            shift
            ;;
        --skip-daemon)
            SKIP_DAEMON=true
            shift
            ;;
        --skip-cli)
            SKIP_CLI=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --skip-ollama    Skip Ollama installation"
            echo "  --skip-daemon    Skip daemon build"
            echo "  --skip-cli       Skip CLI build"
            echo "  --help, -h       Show this help"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# =============================================================================
# Step 1: Prerequisites
# =============================================================================
print_header "Step 1: Prerequisites"
bash "$SETUP_DIR/prerequisites.sh"

# =============================================================================
# Step 2: Platform-Specific Setup
# =============================================================================
if is_macos; then
    print_header "Step 2: Lima VM Setup (macOS)"
    bash "$SETUP_DIR/lima.sh"
else
    print_header "Step 2: runc Setup (Linux)"
    bash "$SETUP_DIR/runc.sh"
fi

# =============================================================================
# Step 3: Build CLI
# =============================================================================
if [[ "$SKIP_CLI" == "false" ]]; then
    print_header "Step 3: Build CLI"
    bash "$SETUP_DIR/cli.sh"
else
    log_info "Skipping CLI build (--skip-cli)"
fi

# =============================================================================
# Step 4: Build Daemon
# =============================================================================
if [[ "$SKIP_DAEMON" == "false" ]]; then
    print_header "Step 4: Build Daemon"
    bash "$SETUP_DIR/daemon.sh"
else
    log_info "Skipping daemon build (--skip-daemon)"
fi

# =============================================================================
# Step 5: Ollama (Optional) - COMMENTED OUT FOR NOW
# =============================================================================
# Ollama setup is not core functionality. Uncomment when needed.
# if [[ "$SKIP_OLLAMA" == "false" ]]; then
#     echo ""
#     if ask_permission "Would you like to set up Ollama for local LLM inference?"; then
#         print_header "Step 5: Ollama Setup"
#         bash "$SETUP_DIR/ollama.sh"
#     else
#         log_info "Skipping Ollama setup"
#     fi
# else
#     log_info "Skipping Ollama setup (--skip-ollama)"
# fi

# =============================================================================
# Summary
# =============================================================================
echo ""
echo -e "${BOLD}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║                                                               ║${NC}"
echo -e "${BOLD}║   ${GREEN}Setup Complete!${NC}${BOLD}                                            ║${NC}"
echo -e "${BOLD}║                                                               ║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""

PROJECT_ROOT=$(get_project_root)

if is_macos; then
    VM_NAME=$(get_lima_vm_name)

    echo -e "${BOLD}Next Steps:${NC}"
    echo ""
    echo "1. Start the daemon (in Lima VM with TCP binding):"
    echo -e "   ${CYAN}limactl shell $VM_NAME${NC}"
    echo -e "   ${CYAN}cd $PROJECT_ROOT && sudo ./bin/orpheusd --tcp-bind 0.0.0.0:7777${NC}"
    echo ""
    echo "2. Connect CLI to daemon (in a new terminal on Mac):"
    echo -e "   ${CYAN}orpheus connect http://localhost:7777${NC}"
    echo ""
    echo "3. Verify connection:"
    echo -e "   ${CYAN}orpheus status${NC}"
    echo ""
    echo "4. Deploy an agent:"
    echo -e "   ${CYAN}orpheus deploy ./examples/basic/calculator-python/${NC}"
    echo ""
    echo "5. Run the agent:"
    echo -e "   ${CYAN}orpheus run calculator-python '{\"expression\": \"2 + 2\"}'${NC}"
    echo ""
else
    echo -e "${BOLD}Next Steps:${NC}"
    echo ""
    echo "1. Start the daemon:"
    echo -e "   ${CYAN}cd $PROJECT_ROOT && sudo ./bin/orpheusd${NC}"
    echo ""
    echo "2. Verify connection (in a new terminal):"
    echo -e "   ${CYAN}orpheus status${NC}"
    echo ""
    echo "3. Deploy an agent:"
    echo -e "   ${CYAN}orpheus deploy ./examples/basic/calculator-python/${NC}"
    echo ""
    echo "4. Run the agent:"
    echo -e "   ${CYAN}orpheus run calculator-python '{\"expression\": \"2 + 2\"}'${NC}"
    echo ""
fi
