#!/bin/bash
# Orpheus Setup - CLI
# Build and link the Orpheus CLI

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

PROJECT_ROOT=$(get_project_root)
CLI_DIR="$PROJECT_ROOT/cli"

print_header "Building Orpheus CLI"

# =============================================================================
# Check Prerequisites
# =============================================================================
log_step "Checking prerequisites"

if ! check_node; then
    log_error "Node.js is required. Run setup/prerequisites.sh first."
    exit 1
fi

if ! check_npm; then
    log_error "npm is required. Run setup/prerequisites.sh first."
    exit 1
fi

# =============================================================================
# Build CLI
# =============================================================================
log_step "Building CLI"

if [[ ! -d "$CLI_DIR" ]]; then
    log_error "CLI directory not found: $CLI_DIR"
    exit 1
fi

cd "$CLI_DIR"

# Install dependencies
log_info "Installing dependencies..."
npm install

# Build
log_info "Building TypeScript..."
npm run build

log_success "CLI built successfully"

# =============================================================================
# Link CLI Globally
# =============================================================================
log_step "Linking CLI globally"

if ask_permission "Link 'orpheus' command globally? (requires sudo)"; then
    log_info "Running npm link..."
    sudo npm link

    # Verify
    if command -v orpheus &>/dev/null; then
        ORPHEUS_VERSION=$(orpheus --version 2>/dev/null || echo "unknown")
        log_success "CLI linked: orpheus $ORPHEUS_VERSION"
    else
        log_warn "CLI linked but 'orpheus' command not found in PATH"
        log_info "Try opening a new terminal or running: hash -r"
    fi
else
    log_info "Skipping global link. Run CLI directly with:"
    echo "  node $CLI_DIR/dist/index.js"
fi

# =============================================================================
# Verify
# =============================================================================
echo ""
log_step "Verifying CLI"

if command -v orpheus &>/dev/null; then
    echo ""
    orpheus --help | head -20
    echo "  ..."
    echo ""
    log_success "CLI is ready!"
else
    log_info "CLI built at: $CLI_DIR/dist/"
    log_info "Run: node $CLI_DIR/dist/index.js --help"
fi
