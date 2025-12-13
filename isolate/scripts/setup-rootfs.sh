#!/bin/bash
# Setup script for minimal rootfs (Alpine Linux)
# This rootfs is used for filesystem isolation with pivot_root

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOTFS_DIR="${SCRIPT_DIR}/../rootfs"
ARCH=$(uname -m)

# Map architecture
if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ALPINE_ARCH="aarch64"
elif [ "$ARCH" = "x86_64" ]; then
    ALPINE_ARCH="x86_64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

echo "[rootfs] Architecture: $ARCH ($ALPINE_ARCH)"
echo "[rootfs] Target directory: $ROOTFS_DIR"

# Create directory
mkdir -p "$ROOTFS_DIR"
cd "$ROOTFS_DIR"

# Alpine version
ALPINE_VERSION="3.19"
ALPINE_RELEASE="${ALPINE_VERSION}.0"

# Download Alpine minirootfs
ROOTFS_URL="https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/releases/${ALPINE_ARCH}/alpine-minirootfs-${ALPINE_RELEASE}-${ALPINE_ARCH}.tar.gz"
ROOTFS_TAR="alpine-minirootfs.tar.gz"

if [ -f "bin/sh" ]; then
    echo "[rootfs] Rootfs already exists"
else
    echo "[rootfs] Downloading Alpine minirootfs..."
    curl -L -o "$ROOTFS_TAR" "$ROOTFS_URL"

    echo "[rootfs] Extracting..."
    tar -xzf "$ROOTFS_TAR"
    rm -f "$ROOTFS_TAR"

    echo "[rootfs] Setting up..."

    # Create necessary directories
    mkdir -p proc sys dev tmp run

    # Set permissions
    chmod 1777 tmp

    echo "[rootfs] Done!"
fi

echo ""
echo "[rootfs] =========================================="
echo "[rootfs] Rootfs ready at: $ROOTFS_DIR"
echo "[rootfs] =========================================="
echo ""
ls -la "$ROOTFS_DIR"
echo ""
echo "To test with pivot_root:"
echo "  sudo ./bin/isolate-linux run --rootfs=$ROOTFS_DIR \"echo hello\""
