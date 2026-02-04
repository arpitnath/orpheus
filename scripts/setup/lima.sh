#!/bin/bash
# Orpheus Setup - Lima VM
# Setup Lima VM for macOS development

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

# =============================================================================
# macOS Check
# =============================================================================
if ! is_macos; then
    log_error "Lima is only required on macOS. Skipping."
    exit 0
fi

print_header "Setting up Lima VM"

# =============================================================================
# Check Lima Installed
# =============================================================================
log_step "Checking Lima installation"
if ! check_lima; then
    log_error "Lima not installed. Run: brew install lima"
    exit 1
fi

# =============================================================================
# Check/Start Lima VM
# =============================================================================
log_step "Checking Lima VM status"

VM_NAME="${LIMA_VM:-default}"

# Get lima list output (capture to avoid SIGPIPE with grep -q + pipefail)
LIMA_OUTPUT=$(limactl list 2>/dev/null || true)

# Check if VM exists
if ! echo "$LIMA_OUTPUT" | grep -q "^$VM_NAME"; then
    log_info "Lima VM '$VM_NAME' does not exist. Creating..."

    if ! ask_permission "Create Lima VM '$VM_NAME'?"; then
        log_error "Lima VM required for local development on macOS."
        exit 1
    fi

    log_info "Creating VM with default Ubuntu template..."
    limactl start "$VM_NAME" --tty=false

    # Wait for VM to be fully running
    log_info "Waiting for VM to be ready..."
    for i in {1..10}; do
        LIMA_OUTPUT=$(limactl list 2>/dev/null || true)
        if echo "$LIMA_OUTPUT" | grep -q "^$VM_NAME.*Running"; then
            break
        fi
        sleep 1
    done
    log_success "Lima VM '$VM_NAME' created and started"
else
    # VM exists, check if running
    if echo "$LIMA_OUTPUT" | grep -q "^$VM_NAME.*Running"; then
        log_success "Lima VM '$VM_NAME' is already running"
    else
        log_info "Lima VM '$VM_NAME' exists but not running. Starting..."
        limactl start "$VM_NAME"
        log_success "Lima VM '$VM_NAME' started"
    fi
fi

# =============================================================================
# Verify Lima VM
# =============================================================================
log_step "Verifying Lima VM"

# Check we can execute commands
if ! limactl shell "$VM_NAME" -- echo "Lima VM OK" &>/dev/null; then
    log_error "Cannot execute commands in Lima VM"
    exit 1
fi

log_success "Lima VM is accessible"

# Get VM info
echo ""
log_info "VM Info:"
limactl shell "$VM_NAME" -- uname -a
echo ""

# =============================================================================
# Install runc in Lima VM
# =============================================================================
log_step "Checking runc in Lima VM"

if limactl shell "$VM_NAME" -- command -v runc &>/dev/null; then
    RUNC_VERSION=$(limactl shell "$VM_NAME" -- runc --version 2>/dev/null | head -1 || echo "unknown")
    log_success "runc already installed in Lima: $RUNC_VERSION"
else
    log_info "runc not found in Lima VM"

    if ask_permission "Install runc in Lima VM?"; then
        log_info "Installing runc..."
        limactl shell "$VM_NAME" -- sudo apt-get update -qq
        limactl shell "$VM_NAME" -- sudo apt-get install -y -qq runc
        log_success "runc installed in Lima VM"
    else
        log_warn "runc is required for running agents. Install it manually."
    fi
fi

# =============================================================================
# Install Podman in Lima VM (for building runtime images)
# =============================================================================
log_step "Checking Podman in Lima VM"

if limactl shell "$VM_NAME" -- command -v podman &>/dev/null; then
    PODMAN_VERSION=$(limactl shell "$VM_NAME" -- podman --version 2>/dev/null | head -1 || echo "unknown")
    log_success "Podman already installed in Lima: $PODMAN_VERSION"
else
    log_info "Podman not found in Lima VM"

    if ask_permission "Install Podman in Lima VM? (required for building runtime images)"; then
        log_info "Installing Podman..."
        limactl shell "$VM_NAME" -- sudo apt-get update -qq
        limactl shell "$VM_NAME" -- sudo apt-get install -y -qq podman
        log_success "Podman installed in Lima VM"
    else
        log_warn "Podman is required for building runtime images. Install it manually."
    fi
fi

# =============================================================================
# Install Runtimes to Fixed Path
# =============================================================================
log_step "Setting up runtimes at /opt/orpheus/runtimes"

PROJECT_ROOT=$(get_project_root)
RUNTIMES_SOURCE="$PROJECT_ROOT/runtimes"
RUNTIMES_TARGET="/opt/orpheus/runtimes"

# Check if runtimes source exists
if [[ ! -d "$RUNTIMES_SOURCE" ]]; then
    log_warn "Runtimes source not found at $RUNTIMES_SOURCE"
    log_info "Skipping runtimes installation. You may need to clone the full repo."
else
    # Create /opt/orpheus directory
    limactl shell "$VM_NAME" -- sudo mkdir -p /opt/orpheus

    # Check if already set up correctly
    CURRENT_LINK=$(limactl shell "$VM_NAME" -- readlink "$RUNTIMES_TARGET" 2>/dev/null || echo "")

    if [[ "$CURRENT_LINK" == "$RUNTIMES_SOURCE" ]]; then
        log_success "Runtimes already symlinked at $RUNTIMES_TARGET"
    else
        # Remove existing (if any) and create symlink
        limactl shell "$VM_NAME" -- sudo rm -rf "$RUNTIMES_TARGET"
        limactl shell "$VM_NAME" -- sudo ln -s "$RUNTIMES_SOURCE" "$RUNTIMES_TARGET"
        log_success "Runtimes symlinked: $RUNTIMES_TARGET -> $RUNTIMES_SOURCE"
    fi

    # Verify the symlink works
    if limactl shell "$VM_NAME" -- test -f "$RUNTIMES_TARGET/runtimes.json"; then
        log_success "Runtimes installation verified"
    else
        log_warn "Runtimes symlink created but runtimes.json not found"
    fi
fi


# =============================================================================
# Install Go in Lima VM (for building daemon)
# =============================================================================
log_step "Checking Go in Lima VM"

if limactl shell "$VM_NAME" -- command -v go &>/dev/null; then
    GO_VERSION=$(limactl shell "$VM_NAME" -- go version 2>/dev/null | awk '{print $3}')
    log_success "Go already installed in Lima: $GO_VERSION"
else
    log_info "Go not found in Lima VM"

    if ask_permission "Install Go in Lima VM? (required for building daemon)"; then
        log_info "Installing Go..."

        # Detect architecture
        ARCH=$(limactl shell "$VM_NAME" -- uname -m)
        if [[ "$ARCH" == "aarch64" ]]; then
            GO_ARCH="arm64"
        else
            GO_ARCH="amd64"
        fi

        GO_VERSION="1.21.6"
        limactl shell "$VM_NAME" -- bash -c "
            curl -fsSL 'https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz' -o /tmp/go.tar.gz
            sudo rm -rf /usr/local/go
            sudo tar -C /usr/local -xzf /tmp/go.tar.gz
            rm /tmp/go.tar.gz
        "

        # Add to PATH in bashrc
        limactl shell "$VM_NAME" -- bash -c "
            if ! grep -q '/usr/local/go/bin' ~/.bashrc; then
                echo 'export PATH=\$PATH:/usr/local/go/bin' >> ~/.bashrc
            fi
        "

        log_success "Go installed in Lima VM"
    else
        log_warn "Go is required for building the daemon. Install it manually."
    fi
fi

# =============================================================================
# Summary
# =============================================================================
echo ""
print_header "Lima Setup Complete"

log_info "Lima VM: $VM_NAME"
log_info "Status: Running"
echo ""

log_info "To enter Lima VM:"
echo "  limactl shell $VM_NAME"
echo ""

log_info "To stop Lima VM:"
echo "  limactl stop $VM_NAME"
echo ""
