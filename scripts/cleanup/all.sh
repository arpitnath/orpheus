#!/bin/bash
# Orpheus Cleanup - Complete Reset
# Clean up EVERYTHING for a fresh start

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

# =============================================================================
# Configuration
# =============================================================================
ORPHEUS_HOME="$HOME/.orpheus"
ORPHEUS_TMP="/tmp/orpheus*"
ORPHEUS_VAR="/var/lib/orpheus"
VM_NAME="${LIMA_VM:-default}"

# Track what was cleaned
CLEANED_ITEMS=()
SKIPPED_ITEMS=()
FAILED_ITEMS=()

# =============================================================================
# Helper Functions
# =============================================================================
add_cleaned() {
    CLEANED_ITEMS+=("$1")
}

add_skipped() {
    SKIPPED_ITEMS+=("$1")
}

add_failed() {
    FAILED_ITEMS+=("$1")
}

show_items_to_delete() {
    local title="$1"
    shift
    local items=("$@")

    if [[ ${#items[@]} -gt 0 ]]; then
        echo -e "${BOLD}$title:${NC}"
        for item in "${items[@]}"; do
            echo "  - $item"
        done
        echo ""
    fi
}

# =============================================================================
# Daemon Cleanup
# =============================================================================
stop_daemon() {
    print_header "Stopping Orpheus Daemon"

    local daemon_stopped=false

    # Check if running inside Lima (macOS)
    if is_macos; then
        # Check if Lima is running
        local lima_output
        lima_output=$(limactl list 2>/dev/null || true)

        if echo "$lima_output" | grep -q "Running"; then
            log_info "Checking for orpheusd inside Lima VM..."

            # Try to stop daemon inside Lima
            local lima_vm
            lima_vm=$(get_lima_vm_name)

            if limactl shell "$lima_vm" -- pgrep -x orpheusd &>/dev/null 2>&1; then
                log_step "Stopping orpheusd inside Lima VM '$lima_vm'..."
                if limactl shell "$lima_vm" -- sudo pkill -x orpheusd 2>/dev/null; then
                    log_success "Stopped orpheusd inside Lima"
                    daemon_stopped=true
                    add_cleaned "orpheusd (Lima VM)"
                else
                    log_warn "Could not stop orpheusd inside Lima"
                    add_failed "orpheusd (Lima VM)"
                fi
            else
                log_info "No orpheusd running inside Lima VM"
            fi
        fi
    fi

    # Check for local daemon (Linux or local macOS process)
    if pgrep -x orpheusd &>/dev/null 2>&1; then
        log_step "Stopping local orpheusd process..."
        if sudo pkill -x orpheusd 2>/dev/null; then
            log_success "Stopped local orpheusd"
            daemon_stopped=true
            add_cleaned "orpheusd (local)"
        else
            log_warn "Could not stop local orpheusd"
            add_failed "orpheusd (local)"
        fi
    else
        log_info "No local orpheusd process running"
    fi

    if ! $daemon_stopped; then
        log_info "No daemon processes were running"
        add_skipped "orpheusd (not running)"
    fi

    # Clean up socket file
    if [[ -S /tmp/orpheus.sock ]]; then
        log_step "Removing daemon socket..."
        sudo rm -f /tmp/orpheus.sock
        log_success "Removed /tmp/orpheus.sock"
        add_cleaned "/tmp/orpheus.sock"
    fi
}

# =============================================================================
# Lima VM Cleanup
# =============================================================================
stop_lima() {
    print_header "Lima VM Cleanup"

    if ! is_macos; then
        log_info "Lima is only available on macOS. Skipping."
        add_skipped "Lima VM (not macOS)"
        return
    fi

    if ! check_command limactl; then
        log_info "Lima is not installed. Skipping."
        add_skipped "Lima VM (not installed)"
        return
    fi

    local lima_output
    lima_output=$(limactl list 2>/dev/null || true)

    # Check if any VM exists
    if ! echo "$lima_output" | grep -qE "^(default|orpheus)"; then
        log_info "No Lima VM found. Skipping."
        add_skipped "Lima VM (not found)"
        return
    fi

    # Show current status
    log_info "Current Lima VM status:"
    echo "$lima_output" | grep -E "^(default|orpheus|NAME)" || true
    echo ""

    # Find running VMs
    local running_vms
    running_vms=$(echo "$lima_output" | grep "Running" | awk '{print $1}' || true)

    if [[ -n "$running_vms" ]]; then
        for vm in $running_vms; do
            log_step "Stopping Lima VM '$vm'..."
            if limactl stop "$vm" 2>/dev/null; then
                log_success "Stopped VM '$vm'"
                add_cleaned "Lima VM '$vm' (stopped)"
            else
                log_warn "Could not stop VM '$vm'"
                add_failed "Lima VM '$vm' (stop)"
            fi
        done
    else
        log_info "No running Lima VMs"
    fi

    # Ask about deleting VM
    echo ""
    local all_vms
    all_vms=$(echo "$lima_output" | grep -E "^(default|orpheus)" | awk '{print $1}' || true)

    if [[ -n "$all_vms" ]]; then
        echo -e "${YELLOW}The following Lima VMs can be deleted:${NC}"
        for vm in $all_vms; do
            echo "  - $vm"
        done
        echo ""

        if ask_permission "Delete Lima VMs? (This will remove all data inside the VMs)"; then
            for vm in $all_vms; do
                log_step "Deleting Lima VM '$vm'..."
                if limactl delete "$vm" --force 2>/dev/null; then
                    log_success "Deleted VM '$vm'"
                    add_cleaned "Lima VM '$vm' (deleted)"
                else
                    log_warn "Could not delete VM '$vm'"
                    add_failed "Lima VM '$vm' (delete)"
                fi
            done
        else
            for vm in $all_vms; do
                add_skipped "Lima VM '$vm' (kept)"
            done
            log_info "Lima VMs kept"
        fi
    fi
}

# =============================================================================
# Orpheus Data Directory Cleanup
# =============================================================================
cleanup_orpheus_home() {
    print_header "Orpheus Home Directory Cleanup"

    if [[ ! -d "$ORPHEUS_HOME" ]]; then
        log_info "Orpheus home directory does not exist: $ORPHEUS_HOME"
        add_skipped "$ORPHEUS_HOME (not found)"
        return
    fi

    # Show what will be deleted
    log_info "Contents of $ORPHEUS_HOME:"
    local items_to_delete=()

    for item in "$ORPHEUS_HOME"/*; do
        if [[ -e "$item" ]]; then
            local item_name
            item_name=$(basename "$item")
            local item_size
            item_size=$(du -sh "$item" 2>/dev/null | cut -f1 || echo "?")
            items_to_delete+=("$item_name ($item_size)")
            echo "  - $item_name ($item_size)"
        fi
    done

    if [[ ${#items_to_delete[@]} -eq 0 ]]; then
        log_info "Directory is empty"
        add_skipped "$ORPHEUS_HOME (empty)"
        return
    fi

    echo ""
    echo -e "${YELLOW}This includes:${NC}"
    echo "  - config/     : CLI and daemon configuration"
    echo "  - registry/   : Agent registry database"
    echo "  - agents/     : Downloaded agent definitions"
    echo "  - cache/      : Cached data and build artifacts"
    echo "  - images/     : Container images and bundles"
    echo ""

    if ask_permission "Delete Orpheus home directory ($ORPHEUS_HOME)?"; then
        log_step "Removing $ORPHEUS_HOME..."
        if rm -rf "$ORPHEUS_HOME"; then
            log_success "Removed $ORPHEUS_HOME"
            add_cleaned "$ORPHEUS_HOME"
        else
            log_error "Failed to remove $ORPHEUS_HOME"
            add_failed "$ORPHEUS_HOME"
        fi
    else
        add_skipped "$ORPHEUS_HOME (kept)"
        log_info "Kept $ORPHEUS_HOME"
    fi
}

# =============================================================================
# Temp Directory Cleanup
# =============================================================================
cleanup_temp() {
    print_header "Temp Directory Cleanup"

    local temp_items
    temp_items=$(ls -d /tmp/orpheus* 2>/dev/null || true)

    if [[ -z "$temp_items" ]]; then
        log_info "No Orpheus temp files found in /tmp"
        add_skipped "/tmp/orpheus* (not found)"
        return
    fi

    log_info "Orpheus temp files found:"
    local total_size=0
    for item in $temp_items; do
        local item_size
        item_size=$(du -sh "$item" 2>/dev/null | cut -f1 || echo "?")
        echo "  - $item ($item_size)"
    done
    echo ""

    if ask_permission "Delete all Orpheus temp files?"; then
        log_step "Removing /tmp/orpheus*..."
        local count=0
        for item in $temp_items; do
            if rm -rf "$item" 2>/dev/null; then
                ((count++))
            fi
        done
        log_success "Removed $count temp item(s)"
        add_cleaned "/tmp/orpheus* ($count items)"
    else
        add_skipped "/tmp/orpheus* (kept)"
        log_info "Kept temp files"
    fi
}

# =============================================================================
# Var Directory Cleanup (requires sudo)
# =============================================================================
cleanup_var() {
    print_header "System Data Directory Cleanup"

    if [[ ! -d "$ORPHEUS_VAR" ]]; then
        log_info "System data directory does not exist: $ORPHEUS_VAR"
        add_skipped "$ORPHEUS_VAR (not found)"
        return
    fi

    log_info "System data directory found: $ORPHEUS_VAR"
    local dir_size
    dir_size=$(sudo du -sh "$ORPHEUS_VAR" 2>/dev/null | cut -f1 || echo "?")
    echo "  Size: $dir_size"
    echo ""

    log_warn "This directory may contain runtime data and requires sudo to delete."
    echo ""

    if ask_permission "Delete system data directory ($ORPHEUS_VAR)? (requires sudo)"; then
        log_step "Removing $ORPHEUS_VAR..."
        if sudo rm -rf "$ORPHEUS_VAR"; then
            log_success "Removed $ORPHEUS_VAR"
            add_cleaned "$ORPHEUS_VAR"
        else
            log_error "Failed to remove $ORPHEUS_VAR"
            add_failed "$ORPHEUS_VAR"
        fi
    else
        add_skipped "$ORPHEUS_VAR (kept)"
        log_info "Kept $ORPHEUS_VAR"
    fi
}

# =============================================================================
# Summary
# =============================================================================
print_cleanup_summary() {
    print_header "Cleanup Summary"

    if [[ ${#CLEANED_ITEMS[@]} -gt 0 ]]; then
        echo -e "${GREEN}Cleaned:${NC}"
        for item in "${CLEANED_ITEMS[@]}"; do
            echo -e "  ${GREEN}[OK]${NC} $item"
        done
        echo ""
    fi

    if [[ ${#SKIPPED_ITEMS[@]} -gt 0 ]]; then
        echo -e "${YELLOW}Skipped:${NC}"
        for item in "${SKIPPED_ITEMS[@]}"; do
            echo -e "  ${YELLOW}[-]${NC} $item"
        done
        echo ""
    fi

    if [[ ${#FAILED_ITEMS[@]} -gt 0 ]]; then
        echo -e "${RED}Failed:${NC}"
        for item in "${FAILED_ITEMS[@]}"; do
            echo -e "  ${RED}[X]${NC} $item"
        done
        echo ""
    fi

    # Final status
    if [[ ${#FAILED_ITEMS[@]} -gt 0 ]]; then
        log_warn "Cleanup completed with some failures"
    elif [[ ${#CLEANED_ITEMS[@]} -gt 0 ]]; then
        log_success "Cleanup completed successfully"
    else
        log_info "Nothing was cleaned (all items skipped or not found)"
    fi

    echo ""
    log_info "To set up Orpheus again, run:"
    echo "  ./scripts/orchestrators/setup-local.sh"
}

# =============================================================================
# Main
# =============================================================================
main() {
    print_header "Orpheus Complete Cleanup"

    log_warn "This script will clean up ALL Orpheus data for a fresh start."
    echo ""
    log_info "The following will be affected:"
    echo "  - Orpheus daemon (orpheusd process)"
    if is_macos; then
        echo "  - Lima VM (stop and optionally delete)"
    fi
    echo "  - $ORPHEUS_HOME (config, registry, agents, cache, images)"
    echo "  - /tmp/orpheus* (sockets, bundles, temp files)"
    echo "  - $ORPHEUS_VAR (system data, if exists)"
    echo ""

    if ! ask_permission "Continue with cleanup?" "n"; then
        log_info "Cleanup cancelled"
        exit 0
    fi

    echo ""

    # Run cleanup steps
    stop_daemon
    stop_lima
    cleanup_orpheus_home
    cleanup_temp
    cleanup_var

    # Show summary
    print_cleanup_summary
}

# Run main
main "$@"
