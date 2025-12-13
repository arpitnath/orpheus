#!/bin/bash
# Setup script for AgentScale VM resources
# Downloads PUI PUI Linux (minimal kernel for Apple Virtualization.framework)

set -euo pipefail

RESOURCE_DIR="${HOME}/.agentscale/vm"
ARCH=$(uname -m)

# Determine architecture
if [ "$ARCH" = "arm64" ]; then
    PUIPUI_ARCH="aarch64"
elif [ "$ARCH" = "x86_64" ]; then
    PUIPUI_ARCH="x86_64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

echo "[setup] Architecture: $ARCH ($PUIPUI_ARCH)"
echo "[setup] Resource directory: $RESOURCE_DIR"

# Create directory
mkdir -p "$RESOURCE_DIR"
cd "$RESOURCE_DIR"

# PUI PUI Linux version (minimal kernel for Apple Virtualization.framework)
PUIPUI_VERSION="1.0.3"

echo ""
echo "[setup] Downloading PUI PUI Linux v${PUIPUI_VERSION}..."
echo "[setup] This is a minimal Linux kernel designed for Apple Virtualization.framework"
echo ""

# Download release tarball
TARBALL_URL="https://github.com/Code-Hex/puipui-linux/releases/download/v${PUIPUI_VERSION}/puipui_linux_v${PUIPUI_VERSION}_${PUIPUI_ARCH}.tar.gz"
TARBALL_FILE="puipui_linux.tar.gz"

echo "[setup] Downloading: $TARBALL_URL"
curl -L -o "$TARBALL_FILE" "$TARBALL_URL"

echo "[setup] Extracting..."
tar -xzf "$TARBALL_FILE"

# The tarball contains Image.gz (kernel) and initramfs.cpio.gz
# Decompress and rename to our expected names
if [ -f "Image.gz" ]; then
    echo "[setup] Decompressing kernel..."
    gunzip Image.gz
    mv Image vmlinuz
    echo "[setup] Extracted: vmlinuz"
elif [ -f "vmlinux" ]; then
    mv vmlinux vmlinuz
    echo "[setup] Extracted: vmlinuz"
fi

if [ -f "initramfs.cpio.gz" ]; then
    mv initramfs.cpio.gz initrd
    echo "[setup] Extracted: initrd (compressed - this is fine)"
fi

# Clean up tarball
rm -f "$TARBALL_FILE"

# Remove old Alpine files if they exist
rm -f alpine-*.iso 2>/dev/null || true
rm -rf rootfs 2>/dev/null || true

echo ""
echo "[setup] =========================================="
echo "[setup] Setup complete!"
echo "[setup] =========================================="
echo ""
echo "Resources installed at: $RESOURCE_DIR"
echo ""
ls -lh "$RESOURCE_DIR"
echo ""
echo "PUI PUI Linux is a minimal kernel specifically designed for"
echo "Apple Virtualization.framework testing."
echo ""
echo "You can now run: isolate vm start"
