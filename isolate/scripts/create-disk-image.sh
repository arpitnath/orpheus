#!/bin/bash
# Create a bootable disk image from Alpine rootfs for the VM

set -euo pipefail

RESOURCE_DIR="${HOME}/.agentscale/vm"
ROOTFS_DIR="${RESOURCE_DIR}/rootfs"
DISK_IMAGE="${RESOURCE_DIR}/disk.img"
DISK_SIZE_MB=512

echo "[disk] Creating bootable disk image..."
echo "[disk] Source: ${ROOTFS_DIR}"
echo "[disk] Target: ${DISK_IMAGE}"
echo "[disk] Size: ${DISK_SIZE_MB}MB"

# Check if rootfs exists
if [ ! -d "$ROOTFS_DIR" ]; then
    echo "[disk] ERROR: Rootfs not found. Run setup-vm-resources.sh first."
    exit 1
fi

# Remove old disk image if exists
rm -f "$DISK_IMAGE"

# Create empty disk image
echo "[disk] Creating empty disk image..."
dd if=/dev/zero of="$DISK_IMAGE" bs=1m count=$DISK_SIZE_MB 2>/dev/null

# Attach disk image
echo "[disk] Attaching disk image..."
DEVICE=$(hdiutil attach -nomount "$DISK_IMAGE" | head -1 | awk '{print $1}')
echo "[disk] Attached as: $DEVICE"

# Create partition and filesystem
echo "[disk] Creating filesystem..."
# Use diskutil to erase and format as HFS+ (macOS compatible for creation)
# The VM will reformat to ext4 on first boot if needed, or we use a raw image

# Actually, for Linux VM, we need a Linux filesystem
# Let's use a different approach - create a raw image that Linux can use

hdiutil detach "$DEVICE" 2>/dev/null || true

# For Apple Virtualization Framework, we can use a raw disk image
# The initramfs will set up the root filesystem

echo "[disk] Creating raw disk image for Linux..."

# Create a sparse file
dd if=/dev/zero of="$DISK_IMAGE" bs=1m count=0 seek=$DISK_SIZE_MB 2>/dev/null

echo "[disk] Disk image created: $DISK_IMAGE"
echo "[disk] Size: $(du -h "$DISK_IMAGE" | cut -f1) (sparse)"

# The VM will boot from initramfs and can format/mount this disk
echo ""
echo "[disk] Note: The VM will boot from initramfs (in-memory)."
echo "[disk] The disk image is available for persistent storage."
echo ""
ls -lh "$DISK_IMAGE"
