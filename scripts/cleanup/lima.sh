#!/bin/bash
# Orpheus Cleanup - Lima VM
# Stop and optionally delete Lima VM

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

# =============================================================================
# macOS Check
# =============================================================================
if ! is_macos; then
    log_error "Lima is only available on macOS."
    exit 0
fi

print_header "Lima VM Cleanup"

VM_NAME="${LIMA_VM:-default}"

# Get lima list output
LIMA_OUTPUT=$(limactl list 2>/dev/null || true)

# Check if VM exists
if ! echo "$LIMA_OUTPUT" | grep -q "^$VM_NAME"; then
    log_info "Lima VM '$VM_NAME' does not exist. Nothing to clean up."
    exit 0
fi

# Show current status
log_info "Current VM status:"
echo "$LIMA_OUTPUT" | grep "^$VM_NAME" || true
echo ""

# =============================================================================
# Stop VM
# =============================================================================
if echo "$LIMA_OUTPUT" | grep -q "^$VM_NAME.*Running"; then
    log_step "Stopping Lima VM '$VM_NAME'..."
    limactl stop "$VM_NAME"
    log_success "VM stopped"
else
    log_info "VM is already stopped"
fi

# =============================================================================
# Delete VM (optional)
# =============================================================================
echo ""
if ask_permission "Delete Lima VM '$VM_NAME'? (This will remove all data inside the VM)"; then
    log_step "Deleting Lima VM '$VM_NAME'..."
    limactl delete "$VM_NAME" --force
    log_success "VM deleted"

    echo ""
    log_info "To recreate, run:"
    echo "  ./scripts/orchestrators/setup-local.sh"
else
    log_info "VM kept. To delete later:"
    echo "  limactl delete $VM_NAME"
fi

echo ""
log_success "Cleanup complete"
