#!/bin/bash
# Build python-3.10 base image for macOS VM
# Creates PUI PUI Linux initrd with Python 3.10 runtime + vsock-agent
#
# Usage: ./build-python-3.10-initrd.sh
# Output: ~/.agentscale/images/python-3.10.initrd.gz

set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
IMAGE_NAME="python-3.10"
PYTHON_VERSION="3.10.19"
PYTHON_BUILD="20251217"

# Directories
HOME_DIR="${HOME}"
CACHE_DIR="${HOME_DIR}/.agentscale/cache"
IMAGES_DIR="${HOME_DIR}/.agentscale/images"
IMAGE_FILE="${IMAGES_DIR}/${IMAGE_NAME}.initrd.gz"
BUILD_DIR="/tmp/agentscale-build-initrd-$$"
VM_DIR="${HOME_DIR}/.agentscale/vm"
VSOCK_AGENT="${PROJECT_DIR}/bin/vsock-agent"

# ============================================================================
# Architecture Detection
# ============================================================================

ARCH=$(uname -m)
if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    PYTHON_ARCH="aarch64"
elif [ "$ARCH" = "x86_64" ]; then
    PYTHON_ARCH="x86_64"
else
    echo "[build] ERROR: Unsupported architecture: $ARCH"
    echo "[build] Supported: x86_64, arm64/aarch64"
    exit 1
fi

# ============================================================================
# URLs
# ============================================================================

PYTHON_FILENAME="cpython-${PYTHON_VERSION}+${PYTHON_BUILD}-${PYTHON_ARCH}-unknown-linux-musl-install_only.tar.gz"
PYTHON_URL="https://github.com/astral-sh/python-build-standalone/releases/download/${PYTHON_BUILD}/${PYTHON_FILENAME}"

# ============================================================================
# Cleanup Handler
# ============================================================================

cleanup() {
    local exit_code=$?
    if [ -d "$BUILD_DIR" ]; then
        echo "[build] Cleaning up temporary build directory..." >&2
        rm -rf "$BUILD_DIR"
    fi
    if [ $exit_code -ne 0 ]; then
        echo "" >&2
        echo "[build] ========================================" >&2
        echo "[build] Build FAILED (exit code: $exit_code)" >&2
        echo "[build] ========================================" >&2
    fi
    exit $exit_code
}

trap cleanup EXIT INT TERM

# ============================================================================
# Banner
# ============================================================================

echo ""
echo "[build] ========================================"
echo "[build] AgentScale Python 3.10 Initrd (macOS)"
echo "[build] ========================================"
echo "[build] Architecture: $ARCH"
echo "[build] Python: ${PYTHON_VERSION}+${PYTHON_BUILD}"
echo "[build] Base: PUI PUI Linux"
echo "[build] Target: $IMAGE_FILE"
echo "[build] ========================================"
echo ""

# ============================================================================
# Functions
# ============================================================================

# Download file with caching
download_with_cache() {
    local url="$1"
    local filename="$2"
    local cache_path="${CACHE_DIR}/${filename}"

    # Check cache first
    if [ -f "$cache_path" ]; then
        echo "[build] Using cached: $filename" >&2
        echo "$cache_path"
        return 0
    fi

    # Download
    echo "[build] Downloading: $filename" >&2
    mkdir -p "$CACHE_DIR"

    if ! curl -L --progress-bar -o "${cache_path}.tmp" "$url"; then
        echo "[build] ERROR: Failed to download from $url" >&2
        echo "[build] Please check your internet connection and try again" >&2
        rm -f "${cache_path}.tmp"
        exit 1
    fi

    # Move to final location
    mv "${cache_path}.tmp" "$cache_path"
    local size=$(du -h "$cache_path" | cut -f1)
    echo "[build] Downloaded: $filename ($size)" >&2
    echo "$cache_path"
}

# ============================================================================
# Main Build Process
# ============================================================================

# Step 1: Prerequisites
echo "[build] Step 1: Checking prerequisites..."

if [ ! -f "$VM_DIR/initrd-puipui" ]; then
    echo "[build] ERROR: PUI PUI Linux initrd not found at $VM_DIR/initrd-puipui"
    echo "[build] Please run: agentscale/isolate/scripts/setup-vm-resources.sh"
    exit 1
fi

if [ ! -f "$VSOCK_AGENT" ]; then
    echo "[build] ERROR: vsock-agent not found at $VSOCK_AGENT"
    echo "[build] Please build it first: cd agentscale/isolate && GOOS=linux GOARCH=arm64 go build -o bin/vsock-agent ./cmd/vsock-agent"
    exit 1
fi

# Verify vsock-agent is Linux ELF binary
if ! file "$VSOCK_AGENT" | grep -qE "ELF.*(aarch64|ARM|x86)"; then
    echo "[build] ERROR: vsock-agent is not a Linux ELF binary"
    echo "[build] Found: $(file "$VSOCK_AGENT")"
    echo "[build] Rebuild for Linux: GOOS=linux GOARCH=arm64 go build -o bin/vsock-agent ./cmd/vsock-agent"
    exit 1
fi

echo "[build] ✓ PUI PUI initrd found"
echo "[build] ✓ vsock-agent found (Linux ARM64)"

# Step 2: Download Python
echo ""
echo "[build] Step 2: Downloading Python runtime..."
PYTHON_TARBALL=$(download_with_cache "$PYTHON_URL" "$PYTHON_FILENAME")
echo "[build] Python: $PYTHON_TARBALL"

# Step 3: Extract base initrd
echo ""
echo "[build] Step 3: Extracting PUI PUI Linux initrd..."
mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

if ! gunzip -c "$VM_DIR/initrd-puipui" | cpio -idm 2>/dev/null; then
    echo "[build] ERROR: Failed to extract initrd"
    exit 1
fi

echo "[build] Base initrd extracted"

# Step 4: Add Python
echo ""
echo "[build] Step 4: Adding Python to initrd..."
mkdir -p python-temp
if ! tar -xzf "$PYTHON_TARBALL" -C python-temp; then
    echo "[build] ERROR: Failed to extract Python tarball"
    exit 1
fi

PYTHON_INSTALL_DIR="${BUILD_DIR}/python-temp/python"
if [ ! -d "$PYTHON_INSTALL_DIR" ]; then
    echo "[build] ERROR: Python directory not found"
    exit 1
fi

# Copy Python files
mkdir -p usr/local/bin usr/local/lib

if [ -d "${PYTHON_INSTALL_DIR}/bin" ]; then
    cp -a "${PYTHON_INSTALL_DIR}/bin"/* usr/local/bin/ 2>/dev/null || true
fi

if [ -d "${PYTHON_INSTALL_DIR}/lib" ]; then
    cp -a "${PYTHON_INSTALL_DIR}/lib"/* usr/local/lib/ 2>/dev/null || true
fi

# Create symlinks
cd usr/local/bin
if [ -f "python3.10" ]; then
    ln -sf python3.10 python3
    ln -sf python3.10 python
fi
cd "$BUILD_DIR"

mkdir -p bin
cd bin
ln -sf ../usr/local/bin/python3 python3
ln -sf ../usr/local/bin/python3 python
cd "$BUILD_DIR"

echo "[build] Python installed"

# Step 5: Add vsock-agent
echo ""
echo "[build] Step 5: Adding vsock-agent..."
cp "$VSOCK_AGENT" ./bin/vsock-agent
chmod +x ./bin/vsock-agent
echo "[build] vsock-agent installed"

# Step 6: Create init script
echo ""
echo "[build] Step 6: Creating init script..."
cat > ./init << 'INIT_EOF'
#!/bin/busybox sh
# AgentScale VM init script with Python 3.10 support

# Install busybox applets
/bin/busybox --install -s /bin

# Create mount points
mkdir -p /proc /sys /dev /dev/pts /tmp

# Mount essential filesystems
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mount -t devpts devpts /dev/pts

# Set hostname
hostname agentscale-vm

# Set up Python environment
export PATH="/usr/local/bin:/bin:/usr/bin:/sbin:/usr/sbin"
export PYTHONHOME="/usr/local"
export PYTHONUNBUFFERED=1

echo ""
echo "==================================="
echo " AgentScale VM (Python 3.10)"
echo "==================================="
echo ""

# Start vsock-agent
echo "[init] Starting vsock-agent..."
/bin/vsock-agent &
VSOCK_PID=$!

echo "[init] vsock-agent started (PID: $VSOCK_PID)"
echo "[init] Python 3.10 available at /usr/local/bin/python3"
echo ""

# Wait for vsock-agent
wait $VSOCK_PID

# Shutdown when vsock-agent exits
echo "[init] vsock-agent exited, shutting down..."
poweroff -f
INIT_EOF

chmod +x ./init
echo "[build] Init script created"

# Step 7: Optimize
echo ""
echo "[build] Step 7: Optimizing size..."
echo "[build] Size before: $(du -sh . | cut -f1)"

find usr/local/lib/python3.10 -type d -name "test" -exec rm -rf {} + 2>/dev/null || true
find usr/local/lib/python3.10 -type d -name "tests" -exec rm -rf {} + 2>/dev/null || true
find usr/local/lib/python3.10 -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
find usr/local/lib/python3.10 -name "*.pyc" -delete 2>/dev/null || true
find usr/local/lib/python3.10 -name "*.pyo" -delete 2>/dev/null || true
rm -rf usr/local/share/doc usr/local/share/man 2>/dev/null || true

echo "[build] Size after: $(du -sh . | cut -f1)"

# Step 8: Repack as initrd
echo ""
echo "[build] Step 8: Repacking as compressed initrd..."
mkdir -p "$IMAGES_DIR"

# Remove old image if exists
if [ -f "$IMAGE_FILE" ]; then
    rm -f "$IMAGE_FILE"
fi

# Repack with cpio + gzip
find . | cpio -H newc -o 2>/dev/null | gzip > "$IMAGE_FILE"

echo "[build] Initrd created: $IMAGE_FILE"
echo "[build] Size: $(du -sh "$IMAGE_FILE" | cut -f1)"

# Step 9: Verification
echo ""
echo "[build] Step 9: Running verification..."

# Test 1: Is gzipped
if ! file "$IMAGE_FILE" | grep -q "gzip"; then
    echo "[test] FAILED: Not a gzip file"
    exit 1
fi
echo "[test] ✓ File is gzipped"

# Test 2: Extract and verify contents
VERIFY_DIR="/tmp/verify-$$"
mkdir -p "$VERIFY_DIR"
cd "$VERIFY_DIR"
gunzip -c "$IMAGE_FILE" | cpio -idm 2>/dev/null

if [ ! -f "usr/local/bin/python3" ]; then
    echo "[test] FAILED: python3 not found"
    rm -rf "$VERIFY_DIR"
    exit 1
fi
echo "[test] ✓ Python binary exists"

if [ ! -f "bin/vsock-agent" ]; then
    echo "[test] FAILED: vsock-agent not found"
    rm -rf "$VERIFY_DIR"
    exit 1
fi
echo "[test] ✓ vsock-agent exists"

if ! grep -q "/usr/local/bin" init; then
    echo "[test] FAILED: Init script missing PATH"
    rm -rf "$VERIFY_DIR"
    exit 1
fi
echo "[test] ✓ Init script configured"

rm -rf "$VERIFY_DIR"

# Success!
echo ""
echo "[build] ========================================"
echo "[build] Build Complete!"
echo "[build] ========================================"
echo "[build] Initrd: $IMAGE_FILE"
echo "[build] Size: $(du -sh "$IMAGE_FILE" | cut -f1)"
echo "[build] Python: ${PYTHON_VERSION}"
echo "[build] ========================================"
echo ""
echo "Initrd ready for macOS VM isolation"
echo ""
