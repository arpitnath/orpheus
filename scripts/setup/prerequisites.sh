#!/bin/bash
# Orpheus Setup - Prerequisites
# Check and install required dependencies

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../lib/common.sh"

print_header "Checking Prerequisites"

OS=$(detect_os)
ARCH=$(detect_arch)
log_info "Platform: $OS/$ARCH"
echo ""

# Track what needs to be installed
MISSING=()

# =============================================================================
# Check Go
# =============================================================================
log_step "Checking Go (1.20+ required)"
if ! check_go; then
    MISSING+=("go")
fi

# =============================================================================
# Check Node.js
# =============================================================================
log_step "Checking Node.js (18+ required)"
if ! check_node; then
    MISSING+=("node")
fi

# =============================================================================
# Check npm
# =============================================================================
log_step "Checking npm"
if ! check_npm; then
    MISSING+=("npm")
fi

# =============================================================================
# Platform-specific checks
# =============================================================================
if is_macos; then
    log_step "Checking Lima (macOS)"
    if ! check_lima; then
        MISSING+=("lima")
    fi

    log_step "Checking Homebrew (macOS)"
    if ! check_brew; then
        MISSING+=("brew")
    fi
fi

if is_linux; then
    log_step "Checking runc (Linux)"
    if ! check_runc; then
        MISSING+=("runc")
    fi
fi

# =============================================================================
# Summary and Install
# =============================================================================
echo ""
if [[ ${#MISSING[@]} -eq 0 ]]; then
    log_success "All prerequisites are installed!"
    exit 0
fi

log_warn "Missing dependencies: ${MISSING[*]}"
echo ""

# Ask user if they want to install
if ! ask_permission "Would you like to install missing dependencies?"; then
    log_info "Skipping installation. Please install manually:"
    for dep in "${MISSING[@]}"; do
        case "$dep" in
            go)
                echo "  - Go: https://go.dev/dl/"
                ;;
            node|npm)
                echo "  - Node.js: https://nodejs.org/"
                ;;
            lima)
                echo "  - Lima: brew install lima"
                ;;
            brew)
                echo "  - Homebrew: https://brew.sh/"
                ;;
            runc)
                echo "  - runc: sudo apt-get install runc"
                ;;
        esac
    done
    exit 1
fi

# =============================================================================
# Install Missing Dependencies
# =============================================================================
echo ""
log_step "Installing missing dependencies..."

for dep in "${MISSING[@]}"; do
    case "$dep" in
        brew)
            log_info "Installing Homebrew..."
            /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
            log_success "Homebrew installed"
            ;;
        go)
            if is_macos && check_brew; then
                log_info "Installing Go via Homebrew..."
                brew install go
                log_success "Go installed"
            elif is_linux; then
                log_info "Installing Go..."
                # Download and install Go
                GO_VERSION="1.21.6"
                if [[ "$ARCH" == "arm64" ]]; then
                    GO_ARCH="arm64"
                else
                    GO_ARCH="amd64"
                fi
                curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -o /tmp/go.tar.gz
                sudo rm -rf /usr/local/go
                sudo tar -C /usr/local -xzf /tmp/go.tar.gz
                rm /tmp/go.tar.gz
                echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
                export PATH=$PATH:/usr/local/go/bin
                log_success "Go installed"
            else
                log_error "Please install Go manually from https://go.dev/dl/"
            fi
            ;;
        node|npm)
            if is_macos && check_brew; then
                log_info "Installing Node.js via Homebrew..."
                brew install node
                log_success "Node.js installed"
            elif is_linux; then
                log_info "Installing Node.js via NodeSource..."
                curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
                sudo apt-get install -y nodejs
                log_success "Node.js installed"
            else
                log_error "Please install Node.js manually from https://nodejs.org/"
            fi
            ;;
        lima)
            if check_brew; then
                log_info "Installing Lima via Homebrew..."
                brew install lima
                log_success "Lima installed"
            else
                log_error "Please install Homebrew first, then run: brew install lima"
            fi
            ;;
        runc)
            if is_linux; then
                log_info "Installing runc..."
                sudo apt-get update
                sudo apt-get install -y runc
                log_success "runc installed"
            else
                log_warn "runc is Linux-only. On macOS, it runs inside Lima VM."
            fi
            ;;
    esac
done

echo ""
log_success "Prerequisites installation complete!"
echo ""

# Verify installations
log_step "Verifying installations..."
check_go || true
check_node || true
check_npm || true
if is_macos; then
    check_lima || true
fi
if is_linux; then
    check_runc || true
fi
