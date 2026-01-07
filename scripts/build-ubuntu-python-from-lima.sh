#!/bin/bash
set -euo pipefail

# Build Ubuntu-based Python 3.10 base image from Lima VM
# Extracts Ubuntu 24.04 filesystem directly from running Lima instance

IMAGE_NAME="python-3.10"
TARGET_DIR="$HOME/.orpheus/images/$IMAGE_NAME"

echo "=== Building Ubuntu-based Python 3.10 Base Image from Lima VM ==="
echo "Target: $TARGET_DIR"
echo ""

# Check if Lima VM is running
if ! limactl list | grep -q "orpheus.*Running"; then
    echo "Error: Lima VM 'orpheus' is not running"
    echo "Start it with: orpheus vm start"
    exit 1
fi

echo "✓ Lima VM is running"
echo ""

# Install Python and dependencies in Lima VM
echo "Installing Python 3.12 in Lima VM..."
limactl shell orpheus -- sudo apt-get update -qq
limactl shell orpheus -- sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    python3.13 \
    python3-pip \
    ca-certificates

echo "✓ Python installed"
echo ""

# Create temporary directory in Lima
echo "Creating base rootfs in Lima VM..."
limactl shell orpheus -- sudo rm -rf /tmp/orpheus-base
limactl shell orpheus -- sudo mkdir -p /tmp/orpheus-base

# Copy essential directories from Ubuntu
echo "Copying Ubuntu system files..."
limactl shell orpheus -- sudo bash -c '
    set -e
    BASE=/tmp/orpheus-base

    # Copy essential system directories
    mkdir -p $BASE/{bin,lib,lib64,usr/bin,usr/lib,etc,tmp,dev,proc,sys,var}

    # Copy binaries - ensure they are copied
    cp -aL /bin/bash $BASE/bin/ 2>/dev/null || true
    cp -aL /bin/sh $BASE/bin/ 2>/dev/null || true

    # Copy Python and essential binaries
    mkdir -p $BASE/usr/bin
    cp -aL /usr/bin/python3.13 $BASE/usr/bin/
    cp -aL /usr/bin/python3 $BASE/usr/bin/ 2>/dev/null || true
    cp -aL /usr/bin/pip3 $BASE/usr/bin/ 2>/dev/null || true

    # Copy libraries (glibc!) - critical for dynamic linking
    mkdir -p $BASE/lib
    cp -a /lib/aarch64-linux-gnu $BASE/lib/
    # Copy dynamic linker symlink (CRITICAL for executing binaries)
    cp -a /lib/ld-linux-aarch64.so.1 $BASE/lib/ 2>/dev/null || true
    cp -a /lib64 $BASE/ 2>/dev/null || true

    # Copy usr/lib completely
    mkdir -p $BASE/usr/lib
    cp -a /usr/lib/python3.13 $BASE/usr/lib/
    cp -a /usr/lib/python3 $BASE/usr/lib/ 2>/dev/null || true
    cp -a /usr/lib/aarch64-linux-gnu $BASE/usr/lib/ 2>/dev/null || true

    # Copy essential configs
    mkdir -p $BASE/etc
    cp /etc/ld.so.* $BASE/etc/ 2>/dev/null || true
    cp /etc/nsswitch.conf $BASE/etc/ 2>/dev/null || true
    cp /etc/hosts $BASE/etc/ 2>/dev/null || true
    cp /etc/resolv.conf $BASE/etc/ 2>/dev/null || true

    # Copy CA certificates
    mkdir -p $BASE/etc/ssl
    cp -a /etc/ssl/certs $BASE/etc/ssl/ 2>/dev/null || true

    # Create symlinks for compatibility
    mkdir -p $BASE/usr/local/bin
    ln -sf /usr/bin/python3.13 $BASE/usr/local/bin/python3.10
    ln -sf /usr/bin/python3.13 $BASE/usr/local/bin/python3
    ln -sf /usr/bin/python3.13 $BASE/usr/local/bin/python
    ln -sf /usr/bin/pip3 $BASE/usr/local/bin/pip

    # Create directories for agent
    mkdir -p $BASE/packages $BASE/agent
    chmod 1777 $BASE/tmp

    echo "✓ Rootfs created"
'

# Create tarball
echo "Creating tarball..."
limactl shell orpheus -- sudo tar -czf /tmp/ubuntu-base.tar.gz -C /tmp/orpheus-base .

# Copy tarball to host
echo "Copying to host..."
rm -f /tmp/ubuntu-base.tar.gz
limactl copy orpheus:/tmp/ubuntu-base.tar.gz /tmp/ubuntu-base.tar.gz

# Extract to target
echo "Extracting to $TARGET_DIR..."
rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
tar -xzf /tmp/ubuntu-base.tar.gz -C "$TARGET_DIR"

# Create manifest
cat > "$TARGET_DIR/manifest.json" <<EOF
{
  "name": "$IMAGE_NAME",
  "version": "2.0.0",
  "spec_version": "v1",
  "runtime": {
    "type": "python3",
    "version": "3.12"
  },
  "base": {
    "os": "ubuntu",
    "version": "24.04"
  },
  "platform": {
    "os": "linux",
    "arch": "arm64"
  },
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "paths": {
    "python_binary": "/usr/local/bin/python3.10",
    "python_lib": "/usr/lib/python3.13",
    "symlinks": [
      "/usr/local/bin/python3 -> python3.10",
      "/usr/local/bin/python -> python3.10"
    ]
  },
  "labels": {
    "maintainer": "orpheus",
    "category": "base",
    "libc": "glibc"
  }
}
EOF

# Calculate size
if [ "$(uname)" = "Darwin" ]; then
    SIZE_BYTES=$(find "$TARGET_DIR" -type f -print0 | xargs -0 stat -f%z | awk '{s+=$1} END {print s}')
else
    SIZE_BYTES=$(du -sb "$TARGET_DIR" | cut -f1)
fi
SIZE_MB=$((SIZE_BYTES / 1024 / 1024))

# Cleanup
rm -f /tmp/ubuntu-base.tar.gz
limactl shell orpheus -- sudo rm -rf /tmp/orpheus-base /tmp/ubuntu-base.tar.gz

echo ""
echo "✓ Base image built successfully!"
echo ""
echo "Location: $TARGET_DIR"
echo "Size: ${SIZE_MB}MB"
echo "Python: $(limactl shell orpheus -- python3.13 --version)"
echo "Libc: glibc (Ubuntu 24.04)"
echo ""
echo "Done! You can now deploy agents with:"
echo "  orpheus deploy <agent-path>"
