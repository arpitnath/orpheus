#!/bin/bash
set -euo pipefail

# Build Ubuntu-based Node.js 20 base image from Lima VM
# Extracts Ubuntu 24.04 filesystem with Node.js 20 LTS directly from running Lima instance

IMAGE_NAME="nodejs-20"
TARGET_DIR="$HOME/.orpheus/images/$IMAGE_NAME"

echo "=== Building Ubuntu-based Node.js 20 Base Image from Lima VM ==="
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

# Install Node.js 20 LTS in Lima VM
echo "Installing Node.js 20 LTS in Lima VM..."
limactl shell orpheus -- sudo apt-get update -qq

# Install Node.js 20 from NodeSource
limactl shell orpheus -- sudo bash -c '
    # Install prerequisites
    apt-get install -y -qq curl ca-certificates gnupg

    # Add NodeSource repository for Node.js 20
    mkdir -p /etc/apt/keyrings
    curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
    echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list

    # Install Node.js 20
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nodejs
'

echo "✓ Node.js installed"
limactl shell orpheus -- node --version
limactl shell orpheus -- npm --version
echo ""

# Create temporary directory in Lima
echo "Creating base rootfs in Lima VM..."
limactl shell orpheus -- sudo rm -rf /tmp/orpheus-nodejs-base
limactl shell orpheus -- sudo mkdir -p /tmp/orpheus-nodejs-base

# Copy essential directories from Ubuntu
echo "Copying Ubuntu system files with Node.js..."
limactl shell orpheus -- sudo bash -c '
    set -e
    BASE=/tmp/orpheus-nodejs-base

    # Copy essential system directories
    mkdir -p $BASE/{bin,lib,lib64,usr/bin,usr/lib,etc,tmp,dev,proc,sys,var}

    # Copy binaries - ensure they are copied
    cp -aL /bin/bash $BASE/bin/ 2>/dev/null || true
    cp -aL /bin/sh $BASE/bin/ 2>/dev/null || true

    # Copy Node.js binaries
    mkdir -p $BASE/usr/bin
    cp -aL /usr/bin/node $BASE/usr/bin/
    cp -aL /usr/bin/npm $BASE/usr/bin/ 2>/dev/null || true
    cp -aL /usr/bin/npx $BASE/usr/bin/ 2>/dev/null || true

    # Copy libraries (glibc!) - critical for dynamic linking
    mkdir -p $BASE/lib
    cp -a /lib/aarch64-linux-gnu $BASE/lib/
    # Copy dynamic linker symlink (CRITICAL for executing binaries)
    cp -a /lib/ld-linux-aarch64.so.1 $BASE/lib/ 2>/dev/null || true
    cp -a /lib64 $BASE/ 2>/dev/null || true

    # Copy usr/lib completely for Node.js
    mkdir -p $BASE/usr/lib
    cp -a /usr/lib/aarch64-linux-gnu $BASE/usr/lib/ 2>/dev/null || true

    # Copy npm global modules if they exist
    if [ -d /usr/lib/node_modules ]; then
        cp -a /usr/lib/node_modules $BASE/usr/lib/
    fi

    # Copy essential configs
    mkdir -p $BASE/etc
    cp /etc/ld.so.* $BASE/etc/ 2>/dev/null || true
    cp /etc/nsswitch.conf $BASE/etc/ 2>/dev/null || true
    cp /etc/hosts $BASE/etc/ 2>/dev/null || true
    cp /etc/resolv.conf $BASE/etc/ 2>/dev/null || true

    # Copy CA certificates (needed for HTTPS)
    mkdir -p $BASE/etc/ssl
    cp -a /etc/ssl/certs $BASE/etc/ssl/ 2>/dev/null || true

    # Create symlinks for compatibility
    mkdir -p $BASE/usr/local/bin
    ln -sf /usr/bin/node $BASE/usr/local/bin/node
    ln -sf /usr/bin/npm $BASE/usr/local/bin/npm
    ln -sf /usr/bin/npx $BASE/usr/local/bin/npx

    # Create directories for agent
    mkdir -p $BASE/packages $BASE/agent $BASE/agent/node_modules
    chmod 1777 $BASE/tmp

    echo "✓ Node.js rootfs created"
'

# Create tarball
echo "Creating tarball..."
limactl shell orpheus -- sudo tar -czf /tmp/nodejs-base.tar.gz -C /tmp/orpheus-nodejs-base .

# Copy tarball to host
echo "Copying to host..."
rm -f /tmp/nodejs-base.tar.gz
limactl copy orpheus:/tmp/nodejs-base.tar.gz /tmp/nodejs-base.tar.gz

# Extract to target
echo "Extracting to $TARGET_DIR..."
rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
tar -xzf /tmp/nodejs-base.tar.gz -C "$TARGET_DIR"

# Get Node.js version
NODE_VERSION=$(limactl shell orpheus -- node --version | tr -d 'v')

# Create manifest
cat > "$TARGET_DIR/manifest.json" <<EOF
{
  "name": "$IMAGE_NAME",
  "version": "1.0.0",
  "spec_version": "v1",
  "runtime": {
    "type": "nodejs20",
    "version": "$NODE_VERSION"
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
    "node_binary": "/usr/local/bin/node",
    "npm_binary": "/usr/local/bin/npm",
    "symlinks": [
      "/usr/local/bin/node -> /usr/bin/node",
      "/usr/local/bin/npm -> /usr/bin/npm"
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
rm -f /tmp/nodejs-base.tar.gz
limactl shell orpheus -- sudo rm -rf /tmp/orpheus-nodejs-base /tmp/nodejs-base.tar.gz

echo ""
echo "✓ Node.js base image built successfully!"
echo ""
echo "Location: $TARGET_DIR"
echo "Size: ${SIZE_MB}MB"
echo "Node.js: v$NODE_VERSION"
echo "Libc: glibc (Ubuntu 24.04)"
echo ""
echo "Done! You can now deploy Node.js agents with:"
echo "  orpheus deploy <agent-path>  # where agent.yaml has runtime: nodejs20"
