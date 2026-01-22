#!/bin/bash
# Orpheus Setup - Ollama
# Install Ollama for local LLM inference

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

print_header "Setting up Ollama"

# =============================================================================
# Platform Detection
# =============================================================================
OS=$(detect_os)

if is_macos; then
    log_info "On macOS, Ollama should run inside Lima VM"
    echo ""

    # Check Lima is running
    if ! check_lima_running; then
        log_error "Lima VM not running. Run setup/lima.sh first."
        exit 1
    fi

    VM_NAME=$(get_lima_vm_name)
    log_info "Using Lima VM: $VM_NAME"

    # Check if Ollama already installed
    if limactl shell "$VM_NAME" -- command -v ollama &>/dev/null; then
        OLLAMA_VERSION=$(limactl shell "$VM_NAME" -- ollama --version 2>/dev/null || echo "unknown")
        log_success "Ollama already installed: $OLLAMA_VERSION"

        # Check if running
        if limactl shell "$VM_NAME" -- pgrep -x ollama &>/dev/null; then
            log_success "Ollama is running"
        else
            log_warn "Ollama is installed but not running"
            if ask_permission "Start Ollama server?"; then
                limactl shell "$VM_NAME" -- bash -c "ollama serve &>/dev/null &"
                sleep 2
                log_success "Ollama server started"
            fi
        fi
        exit 0
    fi

    # Install Ollama in Lima
    if ask_permission "Install Ollama in Lima VM '$VM_NAME'?"; then
        log_info "Installing Ollama..."
        limactl shell "$VM_NAME" -- bash -c "curl -fsSL https://ollama.com/install.sh | sh"
        log_success "Ollama installed"

        # Start Ollama
        if ask_permission "Start Ollama server?"; then
            limactl shell "$VM_NAME" -- bash -c "ollama serve &>/dev/null &"
            sleep 2
            log_success "Ollama server started"
        fi

        # Pull a model
        if ask_permission "Pull 'mistral' model? (~4GB download)"; then
            log_info "Pulling mistral model..."
            limactl shell "$VM_NAME" -- ollama pull mistral
            log_success "Model 'mistral' pulled"
        fi
    else
        log_info "Skipping Ollama installation"
        exit 0
    fi

elif is_linux; then
    log_info "Installing Ollama on Linux"

    # Check if already installed
    if check_ollama; then
        log_success "Ollama already installed"

        # Check if running
        if pgrep -x ollama &>/dev/null; then
            log_success "Ollama is running"
        else
            log_warn "Ollama is installed but not running"
            if ask_permission "Start Ollama server?"; then
                ollama serve &>/dev/null &
                sleep 2
                log_success "Ollama server started"
            fi
        fi
        exit 0
    fi

    # Install Ollama
    if ask_permission "Install Ollama?"; then
        log_info "Installing Ollama..."
        curl -fsSL https://ollama.com/install.sh | sh
        log_success "Ollama installed"

        # Start Ollama
        if ask_permission "Start Ollama server?"; then
            ollama serve &>/dev/null &
            sleep 2
            log_success "Ollama server started"
        fi

        # Pull a model
        if ask_permission "Pull 'mistral' model? (~4GB download)"; then
            log_info "Pulling mistral model..."
            ollama pull mistral
            log_success "Model 'mistral' pulled"
        fi
    else
        log_info "Skipping Ollama installation"
        exit 0
    fi

else
    log_error "Unsupported platform: $OS"
    exit 1
fi

# =============================================================================
# Summary
# =============================================================================
echo ""
print_header "Ollama Setup Complete"

log_info "Ollama is ready for local LLM inference"
echo ""

log_info "To list available models:"
if is_macos; then
    echo "  limactl shell $VM_NAME -- ollama list"
else
    echo "  ollama list"
fi

echo ""
log_info "To pull more models:"
if is_macos; then
    echo "  limactl shell $VM_NAME -- ollama pull <model-name>"
else
    echo "  ollama pull <model-name>"
fi
