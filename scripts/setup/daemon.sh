#!/bin/bash
# Orpheus Setup - Daemon
# Build the Orpheus daemon (orpheusd)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

PROJECT_ROOT=$(get_project_root)
CORE_DIR="$PROJECT_ROOT/core"
BIN_DIR="$PROJECT_ROOT/bin"

print_header "Building Orpheus Daemon"

# =============================================================================
# Platform Detection
# =============================================================================
OS=$(detect_os)

if is_macos; then
    log_info "On macOS, daemon must be built inside Lima VM (Linux target)"
    echo ""

    # Check Lima is running
    if ! check_lima_running; then
        log_error "Lima VM not running. Run setup/lima.sh first."
        exit 1
    fi

    VM_NAME=$(get_lima_vm_name)
    log_info "Using Lima VM: $VM_NAME"

    # Check Go in Lima
    log_step "Checking Go in Lima VM"
    if ! limactl shell "$VM_NAME" -- command -v go &>/dev/null; then
        # Try with explicit path
        if ! limactl shell "$VM_NAME" -- /usr/local/go/bin/go version &>/dev/null; then
            log_error "Go not found in Lima VM. Run setup/lima.sh first."
            exit 1
        fi
    fi

    GO_VERSION=$(limactl shell "$VM_NAME" -- bash -c 'export PATH=$PATH:/usr/local/go/bin && go version' 2>/dev/null | awk '{print $3}')
    log_success "Go in Lima: $GO_VERSION"

    # Build daemon inside Lima
    log_step "Building daemon inside Lima VM"

    # Lima mounts home directory as read-only, so build in /tmp and copy out
    limactl shell "$VM_NAME" -- bash -c "
        export PATH=\$PATH:/usr/local/go/bin
        cd '$CORE_DIR'
        echo 'Building orpheusd...'
        go build -o /tmp/orpheusd ./cmd/orpheusd
    "

    # Copy binary from Lima to host
    log_info "Copying binary to host..."
    mkdir -p "$BIN_DIR"
    limactl copy "$VM_NAME:/tmp/orpheusd" "$BIN_DIR/orpheusd"
    chmod +x "$BIN_DIR/orpheusd"

    if [[ -f "$BIN_DIR/orpheusd" ]]; then
        log_success "Daemon built: $BIN_DIR/orpheusd"

        # Show binary info
        file "$BIN_DIR/orpheusd"
    else
        log_error "Build failed - binary not found"
        exit 1
    fi

elif is_linux; then
    log_info "Building daemon on Linux"

    # Check Go
    log_step "Checking Go"
    if ! check_go; then
        log_error "Go not found. Run setup/prerequisites.sh first."
        exit 1
    fi

    # Build daemon
    log_step "Building daemon"

    cd "$CORE_DIR"
    mkdir -p "$BIN_DIR"

    log_info "Building orpheusd..."
    go build -o "$BIN_DIR/orpheusd" ./cmd/orpheusd

    if [[ -f "$BIN_DIR/orpheusd" ]]; then
        log_success "Daemon built: $BIN_DIR/orpheusd"
        file "$BIN_DIR/orpheusd"
    else
        log_error "Build failed - binary not found"
        exit 1
    fi

else
    log_error "Unsupported platform: $OS"
    exit 1
fi

# =============================================================================
# Summary
# =============================================================================
echo ""
print_header "Daemon Build Complete"

log_info "Binary: $BIN_DIR/orpheusd"
echo ""

if is_macos; then
    log_info "To start daemon (inside Lima VM):"
    echo "  limactl shell $VM_NAME"
    echo "  cd $PROJECT_ROOT"
    echo "  sudo ./bin/orpheusd"
else
    log_info "To start daemon:"
    echo "  cd $PROJECT_ROOT"
    echo "  sudo ./bin/orpheusd"
fi

echo ""
log_info "To verify daemon is running:"
echo "  orpheus status"
