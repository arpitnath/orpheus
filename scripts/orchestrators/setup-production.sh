#!/bin/bash
# Orpheus Setup - Production Self-Hosting
# Main orchestrator for setting up Orpheus on production Linux servers

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
echo -e "${BOLD}║   ${CYAN}ORPHEUS${NC}${BOLD} - Production Self-Hosting Setup                   ║${NC}"
echo -e "${BOLD}║                                                               ║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# =============================================================================
# Platform Detection & Validation
# =============================================================================
OS=$(detect_os)
ARCH=$(detect_arch)

log_info "Detected platform: ${BOLD}$OS/$ARCH${NC}"
echo ""

if [[ "$OS" != "linux" ]]; then
    log_error "This script is for Linux production servers"
    log_info "For macOS local development, use: ./scripts/orchestrators/setup-local.sh"
    exit 1
fi

# Check if running as root or with sudo
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root or with sudo"
    log_info "Run: sudo $0 $@"
    exit 1
fi

# Detect Linux distribution
if [ -f /etc/os-release ]; then
    . /etc/os-release
    DISTRO=$ID
    DISTRO_VERSION=$VERSION_ID
    log_info "Distribution: ${BOLD}$DISTRO $DISTRO_VERSION${NC}"
else
    log_error "Cannot detect Linux distribution"
    exit 1
fi

# Verify supported distribution
case "$DISTRO" in
    ubuntu|debian)
        log_success "Supported distribution: $DISTRO"
        ;;
    *)
        log_warn "Untested distribution: $DISTRO (may work, but not officially supported)"
        read -p "Continue anyway? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
        ;;
esac

echo ""

# =============================================================================
# Parse Arguments
# =============================================================================
SKIP_OLLAMA=false
SKIP_CLI=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-ollama)
            SKIP_OLLAMA=true
            shift
            ;;
        --skip-cli)
            SKIP_CLI=true
            shift
            ;;
        --help|-h)
            echo "Usage: sudo $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --skip-ollama    Skip Ollama installation (model server)"
            echo "  --skip-cli       Skip CLI installation"
            echo "  --help           Show this help message"
            echo ""
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# =============================================================================
# Prerequisites Check
# =============================================================================
log_step "Checking prerequisites"

# Check Go installation
if ! command -v go &>/dev/null; then
    log_warn "Go not found - will install during daemon setup"
else
    GO_VERSION=$(go version | awk '{print $3}')
    log_success "Go installed: $GO_VERSION"
fi

# Check internet connectivity
if ! ping -c 1 -W 2 8.8.8.8 &>/dev/null; then
    log_error "No internet connectivity"
    exit 1
fi

log_success "Prerequisites check passed"
echo ""

# =============================================================================
# Phase 1: Install runc (Container Runtime)
# =============================================================================
print_header "Phase 1/5: Installing runc"

# Check if runc already installed
if command -v runc &>/dev/null; then
    RUNC_VERSION=$(runc --version | head -1)
    log_success "runc already installed: $RUNC_VERSION"
else
    log_step "Installing runc and podman..."

    case "$DISTRO" in
        ubuntu|debian)
            apt-get update -qq
            apt-get install -y -qq runc podman
            ;;
        *)
            log_error "Unsupported distribution for automatic installation"
            log_info "Please install runc and podman manually and re-run this script"
            exit 1
            ;;
    esac

    # Verify runc installation
    if command -v runc &>/dev/null; then
        RUNC_VERSION=$(runc --version | head -1)
        log_success "runc installed: $RUNC_VERSION"
    else
        log_error "runc installation failed"
        exit 1
    fi

    # Verify podman installation
    if command -v podman &>/dev/null; then
        PODMAN_VERSION=$(podman --version)
        log_success "podman installed: $PODMAN_VERSION"
    else
        log_error "podman installation failed"
        exit 1
    fi
fi

echo ""

# =============================================================================
# Install Go (if needed)
# =============================================================================
if ! command -v go &>/dev/null; then
    log_step "Installing Go..."

    GO_VERSION="1.21.6"
    if [[ "$ARCH" == "arm64" ]]; then
        GO_ARCH="arm64"
    else
        GO_ARCH="amd64"
    fi

    cd /tmp
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
    rm "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"

    # Add to PATH
    export PATH=$PATH:/usr/local/go/bin

    # Verify
    if /usr/local/go/bin/go version &>/dev/null; then
        GO_VER=$(/usr/local/go/bin/go version)
        log_success "Go installed: $GO_VER"
    else
        log_error "Go installation failed"
        exit 1
    fi
fi

echo ""

# =============================================================================
# Phase 2: Install Daemon (Orpheus Runtime)
# =============================================================================
print_header "Phase 2/5: Installing Orpheus daemon"

# Ensure Go is in PATH for daemon build
export PATH=$PATH:/usr/local/go/bin

if ! "$SETUP_DIR/daemon.sh"; then
    log_error "Failed to install daemon"
    exit 1
fi

# CRITICAL: Copy built daemon binary to system path
PROJECT_ROOT=$(get_project_root)
DAEMON_BINARY="$PROJECT_ROOT/bin/orpheusd"

if [[ ! -f "$DAEMON_BINARY" ]]; then
    log_error "Daemon binary not found at $DAEMON_BINARY"
    exit 1
fi

log_step "Installing daemon to /usr/local/bin/"
cp "$DAEMON_BINARY" /usr/local/bin/orpheusd
chmod +x /usr/local/bin/orpheusd

# Verify installation
if /usr/local/bin/orpheusd --version &>/dev/null; then
    DAEMON_VERSION=$(/usr/local/bin/orpheusd --version 2>&1 || echo "unknown")
    log_success "Daemon installed: /usr/local/bin/orpheusd ($DAEMON_VERSION)"
else
    log_error "Daemon binary installed but not executable"
    exit 1
fi

# CRITICAL: Setup runtimes directory
log_step "Setting up runtimes at /var/lib/orpheus/runtimes"

RUNTIMES_SOURCE="$PROJECT_ROOT/runtimes"
RUNTIMES_TARGET="/var/lib/orpheus/runtimes"

if [[ ! -d "$RUNTIMES_SOURCE" ]]; then
    log_error "Runtimes source not found at $RUNTIMES_SOURCE"
    log_info "Make sure you cloned the full repository"
    exit 1
fi

# Create /var/lib/orpheus directory (must exist before copying runtimes)
mkdir -p /var/lib/orpheus

# Copy runtimes directory (not symlink, since we're on remote server)
if [[ -d "$RUNTIMES_TARGET" ]]; then
    log_info "Runtimes directory already exists, updating..."
    rm -rf "$RUNTIMES_TARGET"
fi

cp -r "$RUNTIMES_SOURCE" "$RUNTIMES_TARGET"
log_success "Runtimes installed: $RUNTIMES_TARGET"

# Verify runtimes.json exists
if [[ -f "$RUNTIMES_TARGET/runtimes.json" ]]; then
    log_success "Runtimes configuration verified"
else
    log_error "runtimes.json not found after installation"
    exit 1
fi

echo ""

# =============================================================================
# Phase 3: Setup Systemd Service
# =============================================================================
print_header "Phase 3/5: Configuring systemd service"

# Create data directories
log_step "Creating Orpheus data directories"
mkdir -p /var/lib/orpheus/agents
mkdir -p /var/lib/orpheus/workspaces
mkdir -p /var/lib/orpheus/execlog
log_success "Data directories created: /var/lib/orpheus/"

SYSTEMD_SERVICE="/etc/systemd/system/orpheusd.service"

cat > "$SYSTEMD_SERVICE" << 'EOF'
[Unit]
Description=Orpheus Daemon - Agent Runtime
Documentation=https://orpheus.run
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/orpheusd start --tcp-bind 0.0.0.0:8080
Restart=always
RestartSec=5
User=root
WorkingDirectory=/var/lib/orpheus
StandardOutput=journal
StandardError=journal

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable orpheusd
systemctl start orpheusd

# Wait for daemon to start
sleep 2

if systemctl is-active --quiet orpheusd; then
    log_success "Orpheus daemon running"
else
    log_error "Failed to start daemon"
    journalctl -u orpheusd -n 20 --no-pager
    exit 1
fi

echo ""

# =============================================================================
# Phase 4: Install Ollama (Optional Model Server)
# =============================================================================
if [ "$SKIP_OLLAMA" = false ]; then
    print_header "Phase 4/5: Installing Ollama (model server)"

    if ! "$SETUP_DIR/ollama.sh"; then
        log_warn "Ollama installation failed (optional - continuing)"
    fi
else
    log_info "Skipping Ollama installation (--skip-ollama flag)"
fi

echo ""

# =============================================================================
# Phase 5: Install CLI (Optional)
# =============================================================================
if [ "$SKIP_CLI" = false ]; then
    print_header "Phase 5/5: Installing Orpheus CLI"

    # Check if npm is available
    if ! command -v npm &>/dev/null; then
        log_warn "npm not found - install Node.js to use CLI"
        log_info "Daemon is running, you can use HTTP API directly"
    else
        if ! "$SETUP_DIR/cli.sh"; then
            log_warn "CLI installation failed (optional - continuing)"
        fi
    fi
else
    log_info "Skipping CLI installation (--skip-cli flag)"
fi

echo ""

# =============================================================================
# Final Status & Verification
# =============================================================================
print_header "Installation Complete"
echo ""

# Check daemon health
DAEMON_URL="http://localhost:8080"
if curl -sf "$DAEMON_URL/v1/health" &>/dev/null; then
    log_success "Daemon is healthy: $DAEMON_URL"
else
    log_warn "Daemon health check failed (may still be starting)"
fi

# Show status
log_info "Service status:"
systemctl status orpheusd --no-pager -l | head -10

echo ""
log_success "Orpheus setup complete!"
echo ""
echo -e "${BOLD}Next steps:${NC}"
echo "  1. Deploy an agent:    orpheus deploy ./examples/basic/hello-world"
echo "  2. Run the agent:      orpheus run hello-world '{\"message\": \"Hello!\"}'"
echo "  3. View logs:          journalctl -u orpheusd -f"
echo "  4. Check status:       orpheus status"
echo ""
echo -e "${BOLD}Documentation:${NC}"
echo "  - Self-hosting guide:  SELF_HOSTING.md"
echo "  - Troubleshooting:     https://orpheus.run/docs"
echo ""
echo -e "${BOLD}Daemon URL:${NC} $DAEMON_URL"
echo ""
