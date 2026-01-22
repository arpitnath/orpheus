#!/bin/bash
# Orpheus Setup - runc
# Install runc container runtime

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

print_header "Setting up runc"

# =============================================================================
# Platform Detection
# =============================================================================
OS=$(detect_os)

if is_macos; then
    log_info "On macOS, runc runs inside Lima VM"
    log_info "Running setup inside Lima..."
    echo ""

    # Check Lima is running
    if ! check_lima_running; then
        log_error "Lima VM not running. Run setup/lima.sh first."
        exit 1
    fi

    VM_NAME=$(get_lima_vm_name)
    log_info "Using Lima VM: $VM_NAME"

    # Check if runc already installed in Lima
    if limactl shell "$VM_NAME" -- command -v runc &>/dev/null; then
        RUNC_VERSION=$(limactl shell "$VM_NAME" -- runc --version 2>/dev/null | head -1)
        log_success "runc already installed: $RUNC_VERSION"
        exit 0
    fi

    # Install runc in Lima
    if ask_permission "Install runc in Lima VM '$VM_NAME'?"; then
        log_info "Installing runc..."
        limactl shell "$VM_NAME" -- sudo apt-get update -qq
        limactl shell "$VM_NAME" -- sudo apt-get install -y -qq runc

        # Verify
        RUNC_VERSION=$(limactl shell "$VM_NAME" -- runc --version 2>/dev/null | head -1)
        log_success "runc installed: $RUNC_VERSION"
    else
        log_warn "runc is required for running agents."
        exit 1
    fi

elif is_linux; then
    log_info "Installing runc on Linux"

    # Check if already installed
    if check_runc; then
        log_success "runc already installed"
        exit 0
    fi

    # Install runc
    if ask_permission "Install runc?"; then
        log_info "Installing runc..."
        sudo apt-get update -qq
        sudo apt-get install -y -qq runc

        # Verify
        check_runc
        log_success "runc installed"
    else
        log_warn "runc is required for running agents."
        exit 1
    fi

else
    log_error "Unsupported platform: $OS"
    exit 1
fi
