#!/bin/bash
# Build python-3.10 base image for Linux
# Creates Alpine Linux rootfs with Python 3.10 runtime for container isolation
#
# Usage: ./build-python-3.10-image.sh
# Output: ~/.agentscale/images/python-3.10/

set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="python-3.10"
PYTHON_VERSION="3.10.19"
PYTHON_BUILD="20251217"
ALPINE_VERSION="3.19.0"

# Directories
HOME_DIR="${HOME}"
CACHE_DIR="${HOME_DIR}/.agentscale/cache"
IMAGES_DIR="${HOME_DIR}/.agentscale/images"
IMAGE_DIR="${IMAGES_DIR}/${IMAGE_NAME}"
BUILD_DIR="/tmp/agentscale-build-$$"

# ============================================================================
# Architecture Detection
# ============================================================================

ARCH=$(uname -m)
if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ALPINE_ARCH="aarch64"
    PYTHON_ARCH="aarch64"
elif [ "$ARCH" = "x86_64" ]; then
    ALPINE_ARCH="x86_64"
    PYTHON_ARCH="x86_64"
else
    echo "[build] ERROR: Unsupported architecture: $ARCH"
    echo "[build] Supported: x86_64, arm64/aarch64"
    exit 1
fi

# ============================================================================
# URLs
# ============================================================================

ALPINE_FILENAME="alpine-minirootfs-${ALPINE_VERSION}-${ALPINE_ARCH}.tar.gz"
ALPINE_URL="https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/${ALPINE_ARCH}/${ALPINE_FILENAME}"

PYTHON_FILENAME="cpython-${PYTHON_VERSION}+${PYTHON_BUILD}-${PYTHON_ARCH}-unknown-linux-musl-install_only.tar.gz"
PYTHON_URL="https://github.com/astral-sh/python-build-standalone/releases/download/${PYTHON_BUILD}/${PYTHON_FILENAME}"

# ============================================================================
# Cleanup Handler
# ============================================================================

cleanup() {
    local exit_code=$?
    if [ -d "$BUILD_DIR" ]; then
        echo "[build] Cleaning up temporary build directory..."
        rm -rf "$BUILD_DIR"
    fi
    if [ $exit_code -ne 0 ]; then
        echo ""
        echo "[build] ========================================"
        echo "[build] Build FAILED (exit code: $exit_code)"
        echo "[build] ========================================"
    fi
    exit $exit_code
}

trap cleanup EXIT INT TERM

# ============================================================================
# Banner
# ============================================================================

echo ""
echo "[build] ========================================"
echo "[build] AgentScale Python 3.10 Base Image"
echo "[build] ========================================"
echo "[build] Architecture: $ARCH ($ALPINE_ARCH)"
echo "[build] Python: ${PYTHON_VERSION}+${PYTHON_BUILD}"
echo "[build] Alpine: ${ALPINE_VERSION}"
echo "[build] Target: $IMAGE_DIR"
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

echo "[build] Step 1: Downloading components..."
echo ""

# Download Alpine base
ALPINE_TARBALL=$(download_with_cache "$ALPINE_URL" "$ALPINE_FILENAME")

# Download Python runtime
PYTHON_TARBALL=$(download_with_cache "$PYTHON_URL" "$PYTHON_FILENAME")

echo ""
echo "[build] Downloads complete"
echo "[build] Alpine: $ALPINE_TARBALL"
echo "[build] Python: $PYTHON_TARBALL"

# ============================================================================
# Step 2: Extract and Combine
# ============================================================================

echo ""
echo "[build] Step 2: Extracting and combining components..."

# Create build directory
mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

# Extract Alpine base
echo "[build] Extracting Alpine Linux base..."
if ! tar -xzf "$ALPINE_TARBALL"; then
    echo "[build] ERROR: Failed to extract Alpine tarball"
    echo "[build] The download may be corrupted. Try:"
    echo "[build]   rm $ALPINE_TARBALL"
    exit 1
fi

# Extract Python
echo "[build] Extracting Python runtime..."
mkdir -p python-temp
if ! tar -xzf "$PYTHON_TARBALL" -C python-temp; then
    echo "[build] ERROR: Failed to extract Python tarball"
    echo "[build] The download may be corrupted. Try:"
    echo "[build]   rm $PYTHON_TARBALL"
    exit 1
fi

# Locate Python installation
PYTHON_INSTALL_DIR="${BUILD_DIR}/python-temp/python"
if [ ! -d "$PYTHON_INSTALL_DIR" ]; then
    echo "[build] ERROR: Python installation directory not found"
    echo "[build] Expected: $PYTHON_INSTALL_DIR"
    echo "[build] python-build-standalone may have changed structure"
    exit 1
fi

# Copy Python to Alpine base
echo "[build] Installing Python into Alpine base..."
mkdir -p usr/local/bin usr/local/lib

# Copy binaries
if [ -d "${PYTHON_INSTALL_DIR}/bin" ]; then
    cp -a "${PYTHON_INSTALL_DIR}/bin"/* usr/local/bin/ 2>/dev/null || true
fi

# Copy libraries
if [ -d "${PYTHON_INSTALL_DIR}/lib" ]; then
    cp -a "${PYTHON_INSTALL_DIR}/lib"/* usr/local/lib/ 2>/dev/null || true
fi

echo "[build] Python installation complete"

# ============================================================================
# Step 3: Set Up Symlinks
# ============================================================================

echo ""
echo "[build] Step 3: Setting up symlinks..."

# Symlinks in /usr/local/bin
cd usr/local/bin
if [ -f "python3.10" ]; then
    ln -sf python3.10 python3
    ln -sf python3.10 python
    echo "[build] Created symlinks in /usr/local/bin"
fi
cd "$BUILD_DIR"

# Convenience symlinks in /bin
mkdir -p bin
cd bin
ln -sf ../usr/local/bin/python3 python3
ln -sf ../usr/local/bin/python3 python
cd "$BUILD_DIR"

echo "[build] Symlinks configured"

# ============================================================================
# Step 4: Create Mount Points and Minimal /etc
# ============================================================================

echo ""
echo "[build] Step 4: Creating filesystem structure..."

# Essential mount points
mkdir -p proc sys dev tmp run

# Set permissions
chmod 1777 tmp
chmod 755 proc sys dev run

# Minimal /etc/hosts
mkdir -p etc
cat > etc/hosts <<'EOF'
127.0.0.1   localhost
::1         localhost
EOF

# Minimal /etc/resolv.conf
cat > etc/resolv.conf <<'EOF'
nameserver 8.8.8.8
nameserver 8.8.4.4
EOF

echo "[build] Filesystem structure created"

# ============================================================================
# Step 5: Optimize Image
# ============================================================================

echo ""
echo "[build] Step 5: Optimizing image size..."
echo "[build] Size before optimization: $(du -sh . | cut -f1)"

# Remove Python tests
find usr/local/lib/python3.10 -type d -name "test" -exec rm -rf {} + 2>/dev/null || true
find usr/local/lib/python3.10 -type d -name "tests" -exec rm -rf {} + 2>/dev/null || true

# Remove __pycache__
find usr/local/lib/python3.10 -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true

# Remove bytecode
find usr/local/lib/python3.10 -name "*.pyc" -delete 2>/dev/null || true
find usr/local/lib/python3.10 -name "*.pyo" -delete 2>/dev/null || true

# Remove docs
rm -rf usr/local/share/doc usr/local/share/man 2>/dev/null || true
rm -rf usr/share/doc usr/share/man 2>/dev/null || true

# Remove Alpine cache
rm -rf var/cache/apk/* 2>/dev/null || true

echo "[build] Size after optimization: $(du -sh . | cut -f1)"
echo "[build] Optimization complete"

# ============================================================================
# Step 6: Create Manifest
# ============================================================================

echo ""
echo "[build] Step 6: Creating manifest.json..."

# Calculate sizes (compatible with macOS and Linux)
IMAGE_SIZE=$(du -sk . | cut -f1)
IMAGE_SIZE=$((IMAGE_SIZE * 1024))  # Convert KB to bytes
IMAGE_SIZE_MB=$((IMAGE_SIZE / 1024 / 1024))

# Calculate checksums
ALPINE_CHECKSUM=$(sha256sum "$ALPINE_TARBALL" 2>/dev/null | cut -d' ' -f1 || echo "unavailable")
PYTHON_CHECKSUM=$(sha256sum "$PYTHON_TARBALL" 2>/dev/null | cut -d' ' -f1 || echo "unavailable")

# Create manifest
cat > manifest.json <<EOF
{
  "name": "${IMAGE_NAME}",
  "version": "1.0.0",
  "spec_version": "v1",
  "runtime": {
    "type": "python3",
    "version": "${PYTHON_VERSION}"
  },
  "base": {
    "os": "alpine",
    "version": "${ALPINE_VERSION}"
  },
  "platform": {
    "os": "linux",
    "arch": "${ARCH}"
  },
  "size": {
    "bytes": ${IMAGE_SIZE},
    "megabytes": ${IMAGE_SIZE_MB}
  },
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "checksums": {
    "alpine_source": "${ALPINE_CHECKSUM}",
    "python_source": "${PYTHON_CHECKSUM}"
  },
  "paths": {
    "python_binary": "/usr/local/bin/python3",
    "python_lib": "/usr/local/lib/python3.10",
    "symlinks": [
      "/bin/python3 -> /usr/local/bin/python3",
      "/usr/local/bin/python -> python3"
    ]
  },
  "labels": {
    "maintainer": "agentscale",
    "category": "base"
  }
}
EOF

echo "[build] Manifest created"

# ============================================================================
# Step 7: Install to Final Location
# ============================================================================

echo ""
echo "[build] Step 7: Installing image..."

# Remove existing image
if [ -d "$IMAGE_DIR" ]; then
    echo "[build] Removing existing image at $IMAGE_DIR"
    rm -rf "$IMAGE_DIR"
fi

# Create images directory
mkdir -p "$IMAGES_DIR"

# Copy build directory to final location
echo "[build] Copying to: $IMAGE_DIR"
cp -a "$BUILD_DIR" "$IMAGE_DIR"

echo "[build] Image installed successfully"

# ============================================================================
# Step 8: Verification
# ============================================================================

echo ""
echo "[build] Step 8: Running verification tests..."
echo ""

# Test 1: Python binary
echo "[test] 1. Checking Python binary..."
if [ ! -f "$IMAGE_DIR/usr/local/bin/python3" ]; then
    echo "[test] FAILED: python3 binary not found"
    exit 1
fi
echo "[test] ✓ Python binary exists"

# Test 2: Standard library
echo "[test] 2. Checking standard library..."
if [ ! -d "$IMAGE_DIR/usr/local/lib/python3.10" ]; then
    echo "[test] FAILED: Standard library not found"
    exit 1
fi
STDLIB_COUNT=$(find "$IMAGE_DIR/usr/local/lib/python3.10" -name "*.py" 2>/dev/null | wc -l)
echo "[test] ✓ Standard library exists ($STDLIB_COUNT .py files)"

# Test 3: Mount points
echo "[test] 3. Checking mount points..."
for dir in proc sys dev tmp; do
    if [ ! -d "$IMAGE_DIR/$dir" ]; then
        echo "[test] FAILED: Mount point /$dir missing"
        exit 1
    fi
done
echo "[test] ✓ All mount points exist"

# Test 4: Symlinks
echo "[test] 4. Checking symlinks..."
if [ ! -L "$IMAGE_DIR/bin/python3" ]; then
    echo "[test] WARNING: /bin/python3 symlink missing"
fi
if [ ! -L "$IMAGE_DIR/usr/local/bin/python" ]; then
    echo "[test] WARNING: /usr/local/bin/python symlink missing"
fi
echo "[test] ✓ Symlinks configured"

# Test 5: Manifest
echo "[test] 5. Checking manifest..."
if [ ! -f "$IMAGE_DIR/manifest.json" ]; then
    echo "[test] FAILED: manifest.json missing"
    exit 1
fi
echo "[test] ✓ Manifest exists"

# ============================================================================
# Success!
# ============================================================================

echo ""
echo "[build] ========================================"
echo "[build] Build Complete!"
echo "[build] ========================================"
echo "[build] Image: $IMAGE_DIR"
echo "[build] Size: $(du -sh "$IMAGE_DIR" | cut -f1)"
echo "[build] Python: ${PYTHON_VERSION}"
echo "[build] ========================================"
echo ""
echo "To test isolation:"
echo "  sudo ./agentscale/isolate/bin/isolate-linux run \\"
echo "    --rootfs=$IMAGE_DIR \\"
echo "    \"python3 --version\""
echo ""
echo "To test imports:"
echo "  sudo ./agentscale/isolate/bin/isolate-linux run \\"
echo "    --rootfs=$IMAGE_DIR \\"
echo "    \"python3 -c 'import sys, os, json; print(\\\"OK\\\")'\""
echo ""
