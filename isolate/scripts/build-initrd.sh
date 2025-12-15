#!/bin/bash
# Build custom initrd with vsock-agent for AgentScale VM isolation
#
# This script:
# 1. Extracts the PUI PUI Linux initrd
# 2. Adds our vsock-agent binary
# 3. Modifies /init to start vsock-agent on boot
# 4. Repacks the initrd

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
VM_DIR="${HOME}/.agentscale/vm"
WORK_DIR="${PROJECT_DIR}/build/initrd-work"
VSOCK_AGENT="${PROJECT_DIR}/bin/vsock-agent"

echo "[build-initrd] Building custom initrd with vsock-agent"
echo "[build-initrd] Project dir: $PROJECT_DIR"
echo "[build-initrd] VM dir: $VM_DIR"

# Check prerequisites
if [ ! -f "$VM_DIR/initrd-puipui" ] && [ ! -f "$VM_DIR/initrd" ]; then
    echo "[build-initrd] ERROR: No base initrd found. Run setup-vm-resources.sh first."
    exit 1
fi

# Build vsock-agent if needed
if [ ! -f "$VSOCK_AGENT" ]; then
    echo "[build-initrd] Building vsock-agent..."
    cd "$PROJECT_DIR"
    GOOS=linux GOARCH=arm64 go build -o bin/vsock-agent ./cmd/vsock-agent
fi

# Verify vsock-agent exists and is Linux binary
if ! file "$VSOCK_AGENT" | grep -q "ELF.*Linux"; then
    echo "[build-initrd] ERROR: vsock-agent is not a Linux binary. Rebuilding..."
    cd "$PROJECT_DIR"
    GOOS=linux GOARCH=arm64 go build -o bin/vsock-agent ./cmd/vsock-agent
fi

# Clean and create work directory
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"

# Determine source initrd (prefer puipui original if available)
if [ -f "$VM_DIR/initrd-puipui" ]; then
    SOURCE_INITRD="$VM_DIR/initrd-puipui"
else
    SOURCE_INITRD="$VM_DIR/initrd"
fi

echo "[build-initrd] Using base initrd: $SOURCE_INITRD"

# Extract initrd (it's a gzipped cpio archive)
echo "[build-initrd] Extracting initrd..."
gunzip -c "$SOURCE_INITRD" | cpio -idm 2>/dev/null

# Show what we have
echo "[build-initrd] Contents of extracted initrd:"
ls -la

# Copy vsock-agent to /bin
echo "[build-initrd] Adding vsock-agent..."
cp "$VSOCK_AGENT" ./bin/vsock-agent
chmod +x ./bin/vsock-agent

# Read current init script
echo "[build-initrd] Reading current init script..."
if [ -f ./init ]; then
    cat ./init
    echo ""
fi

# Create new init script that starts vsock-agent before dropping to shell
echo "[build-initrd] Creating new init script..."
cat > ./init << 'INIT_EOF'
#!/bin/busybox sh
# AgentScale VM init script
# Starts vsock-agent for command execution from host

# Install busybox applets
/bin/busybox --install -s /bin

# Create mount points if they don't exist
mkdir -p /proc /sys /dev /dev/pts /tmp

# Mount essential filesystems
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev
mount -t devpts devpts /dev/pts

# Set hostname
hostname agentscale-vm

echo ""
echo "==================================="
echo " AgentScale VM"
echo "==================================="
echo ""

# Start vsock-agent in background
echo "[init] Starting vsock-agent..."
/bin/vsock-agent &
VSOCK_PID=$!

echo "[init] vsock-agent started (PID: $VSOCK_PID)"
echo "[init] Listening on vsock port 1024"
echo ""

# Keep the VM running - just wait for the vsock-agent
# If vsock-agent dies, we also exit
wait $VSOCK_PID

# If we get here, vsock-agent exited
echo "[init] vsock-agent exited, shutting down..."
poweroff -f
INIT_EOF

chmod +x ./init

# Repack initrd
echo "[build-initrd] Repacking initrd..."
find . | cpio -H newc -o 2>/dev/null | gzip > "$VM_DIR/initrd"

echo "[build-initrd] Custom initrd created at: $VM_DIR/initrd"
echo "[build-initrd] Size: $(ls -lh "$VM_DIR/initrd" | awk '{print $5}')"

# Cleanup
cd /
rm -rf "$WORK_DIR"

echo ""
echo "[build-initrd] Done!"
echo "[build-initrd] The VM will now start vsock-agent automatically on boot."
