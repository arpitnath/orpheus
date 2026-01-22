#!/bin/bash
# Orpheus Scripts - Common Library
# Shared functions for all scripts

set -euo pipefail

# =============================================================================
# Colors
# =============================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# =============================================================================
# Logging
# =============================================================================
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${CYAN}==>${NC} ${BOLD}$1${NC}"
}

# =============================================================================
# OS Detection
# =============================================================================
detect_os() {
    case "$(uname -s)" in
        Darwin)
            echo "macos"
            ;;
        Linux)
            echo "linux"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)
            echo "amd64"
            ;;
        arm64|aarch64)
            echo "arm64"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

is_macos() {
    [[ "$(detect_os)" == "macos" ]]
}

is_linux() {
    [[ "$(detect_os)" == "linux" ]]
}

# =============================================================================
# User Interaction
# =============================================================================
ask_permission() {
    local prompt="$1"
    local default="${2:-n}"

    if [[ "$default" == "y" ]]; then
        prompt="$prompt [Y/n]"
    else
        prompt="$prompt [y/N]"
    fi

    echo -en "${YELLOW}$prompt ${NC}"
    read -r response

    if [[ -z "$response" ]]; then
        response="$default"
    fi

    [[ "$response" =~ ^[Yy]$ ]]
}

# =============================================================================
# Prerequisite Checks
# =============================================================================
check_command() {
    local cmd="$1"
    command -v "$cmd" &> /dev/null
}

check_go() {
    if check_command go; then
        local version
        version=$(go version | awk '{print $3}' | sed 's/go//')
        log_success "Go installed: v$version"
        return 0
    else
        log_warn "Go not found"
        return 1
    fi
}

check_node() {
    if check_command node; then
        local version
        version=$(node --version)
        log_success "Node.js installed: $version"
        return 0
    else
        log_warn "Node.js not found"
        return 1
    fi
}

check_npm() {
    if check_command npm; then
        local version
        version=$(npm --version)
        log_success "npm installed: v$version"
        return 0
    else
        log_warn "npm not found"
        return 1
    fi
}

check_lima() {
    if check_command limactl; then
        local version
        version=$(limactl --version 2>/dev/null | head -1 || echo "unknown")
        log_success "Lima installed: $version"
        return 0
    else
        log_warn "Lima not found"
        return 1
    fi
}

check_lima_running() {
    # Note: Using variable capture instead of pipe with grep -q
    # because grep -q + pipefail causes SIGPIPE (exit 141)
    local output
    output=$(limactl list 2>/dev/null || true)
    if echo "$output" | grep -q "Running"; then
        local vm_name
        vm_name=$(echo "$output" | grep "Running" | awk '{print $1}' | head -1)
        log_success "Lima VM running: $vm_name"
        return 0
    else
        log_warn "No Lima VM running"
        return 1
    fi
}

check_runc() {
    if check_command runc; then
        local version
        version=$(runc --version 2>/dev/null | head -1 || echo "unknown")
        log_success "runc installed: $version"
        return 0
    else
        log_warn "runc not found"
        return 1
    fi
}

check_ollama() {
    if check_command ollama; then
        local version
        version=$(ollama --version 2>/dev/null || echo "unknown")
        log_success "Ollama installed: $version"
        return 0
    else
        log_warn "Ollama not found"
        return 1
    fi
}

check_brew() {
    if check_command brew; then
        log_success "Homebrew installed"
        return 0
    else
        log_warn "Homebrew not found"
        return 1
    fi
}

# =============================================================================
# Path Helpers
# =============================================================================
get_script_dir() {
    cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
}

get_project_root() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    # Go up from lib/ to scripts/ to project root
    cd "$script_dir/../.." && pwd
}

# =============================================================================
# Lima Helpers
# =============================================================================
lima_exec() {
    local vm_name="${LIMA_VM:-default}"
    limactl shell "$vm_name" -- "$@"
}

lima_exec_sudo() {
    local vm_name="${LIMA_VM:-default}"
    limactl shell "$vm_name" -- sudo "$@"
}

get_lima_vm_name() {
    # Try to find a running VM, prefer 'orpheus' or 'default'
    if limactl list 2>/dev/null | grep -q "orpheus.*Running"; then
        echo "orpheus"
    elif limactl list 2>/dev/null | grep -q "default.*Running"; then
        echo "default"
    else
        # Return first running VM or 'default' if none
        local running
        running=$(limactl list 2>/dev/null | grep "Running" | awk '{print $1}' | head -1)
        echo "${running:-default}"
    fi
}

# =============================================================================
# Daemon Helpers
# =============================================================================
check_daemon_running() {
    if [[ -S /tmp/orpheus.sock ]]; then
        log_success "Daemon socket exists: /tmp/orpheus.sock"
        return 0
    else
        log_warn "Daemon not running (no socket at /tmp/orpheus.sock)"
        return 1
    fi
}

# =============================================================================
# Summary Helpers
# =============================================================================
print_header() {
    local title="$1"
    echo ""
    echo -e "${BOLD}============================================${NC}"
    echo -e "${BOLD}  $title${NC}"
    echo -e "${BOLD}============================================${NC}"
    echo ""
}

print_summary() {
    local title="$1"
    shift
    echo ""
    echo -e "${BOLD}--- $title ---${NC}"
    for item in "$@"; do
        echo "  $item"
    done
    echo ""
}
