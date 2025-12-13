// Package rootfs handles filesystem isolation using pivot_root
package rootfs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Setup prepares the root filesystem for the container
// It creates necessary mount points and pivots to the new root
func Setup(newRoot string) error {
	// Ensure newRoot exists and is a directory
	if info, err := os.Stat(newRoot); err != nil {
		return fmt.Errorf("rootfs path does not exist: %s", newRoot)
	} else if !info.IsDir() {
		return fmt.Errorf("rootfs path is not a directory: %s", newRoot)
	}

	// Make the mount namespace private so our mounts don't propagate
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root private: %w", err)
	}

	// Bind mount the new root to itself (required for pivot_root)
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount new root: %w", err)
	}

	// Create a directory for the old root
	oldRoot := filepath.Join(newRoot, ".old_root")
	if err := os.MkdirAll(oldRoot, 0700); err != nil {
		return fmt.Errorf("failed to create old root directory: %w", err)
	}

	// pivot_root swaps the root filesystem
	// After this, newRoot becomes / and old / is at /.old_root
	if err := syscall.PivotRoot(newRoot, oldRoot); err != nil {
		return fmt.Errorf("pivot_root failed: %w", err)
	}

	// Change to the new root directory
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root: %w", err)
	}

	// Mount essential filesystems
	if err := MountEssentials(); err != nil {
		return fmt.Errorf("failed to mount essentials: %w", err)
	}

	// Unmount and remove the old root
	oldRootNew := "/.old_root"
	if err := syscall.Unmount(oldRootNew, syscall.MNT_DETACH); err != nil {
		// Non-fatal, but log it
		fmt.Printf("[rootfs] Warning: failed to unmount old root: %v\n", err)
	}

	// Remove the old root directory
	if err := os.RemoveAll(oldRootNew); err != nil {
		// Non-fatal
		fmt.Printf("[rootfs] Warning: failed to remove old root: %v\n", err)
	}

	return nil
}

// MountEssentials mounts /proc, /sys, /dev and other essential filesystems
func MountEssentials() error {
	// Mount /proc (required for process info, ps command, etc.)
	if err := os.MkdirAll("/proc", 0555); err != nil {
		return fmt.Errorf("failed to create /proc: %w", err)
	}
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %w", err)
	}

	// Mount /sys (required for cgroups, device info)
	if err := os.MkdirAll("/sys", 0555); err != nil {
		return fmt.Errorf("failed to create /sys: %w", err)
	}
	if err := syscall.Mount("sysfs", "/sys", "sysfs", 0, ""); err != nil {
		// Non-fatal on some minimal systems
		fmt.Printf("[rootfs] Warning: failed to mount /sys: %v\n", err)
	}

	// Mount /dev as tmpfs
	if err := os.MkdirAll("/dev", 0755); err != nil {
		return fmt.Errorf("failed to create /dev: %w", err)
	}
	if err := syscall.Mount("tmpfs", "/dev", "tmpfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755,size=65536k"); err != nil {
		return fmt.Errorf("failed to mount /dev: %w", err)
	}

	// Create essential device nodes
	if err := CreateDeviceNodes(); err != nil {
		return fmt.Errorf("failed to create device nodes: %w", err)
	}

	// Mount /dev/pts for pseudo-terminals
	if err := os.MkdirAll("/dev/pts", 0755); err != nil {
		return fmt.Errorf("failed to create /dev/pts: %w", err)
	}
	if err := syscall.Mount("devpts", "/dev/pts", "devpts", 0, "newinstance,ptmxmode=0666"); err != nil {
		// Non-fatal
		fmt.Printf("[rootfs] Warning: failed to mount /dev/pts: %v\n", err)
	}

	// Mount /dev/shm for shared memory
	if err := os.MkdirAll("/dev/shm", 1777); err != nil {
		return fmt.Errorf("failed to create /dev/shm: %w", err)
	}
	if err := syscall.Mount("shm", "/dev/shm", "tmpfs", syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777,size=65536k"); err != nil {
		// Non-fatal
		fmt.Printf("[rootfs] Warning: failed to mount /dev/shm: %v\n", err)
	}

	// Create /tmp
	if err := os.MkdirAll("/tmp", 1777); err != nil {
		return fmt.Errorf("failed to create /tmp: %w", err)
	}

	return nil
}

// CreateDeviceNodes creates essential device nodes in /dev
func CreateDeviceNodes() error {
	devices := []struct {
		path  string
		mode  uint32
		major uint32
		minor uint32
	}{
		{"/dev/null", syscall.S_IFCHR | 0666, 1, 3},
		{"/dev/zero", syscall.S_IFCHR | 0666, 1, 5},
		{"/dev/full", syscall.S_IFCHR | 0666, 1, 7},
		{"/dev/random", syscall.S_IFCHR | 0666, 1, 8},
		{"/dev/urandom", syscall.S_IFCHR | 0666, 1, 9},
		{"/dev/tty", syscall.S_IFCHR | 0666, 5, 0},
	}

	for _, dev := range devices {
		// Remove if exists
		os.Remove(dev.path)

		// Create device node
		devNum := int(dev.major<<8 | dev.minor)
		if err := syscall.Mknod(dev.path, dev.mode, devNum); err != nil {
			// Try symlink as fallback (some systems don't allow mknod)
			fmt.Printf("[rootfs] Warning: mknod %s failed: %v\n", dev.path, err)
		}
	}

	// Create symlinks
	symlinks := []struct {
		oldname string
		newname string
	}{
		{"/proc/self/fd", "/dev/fd"},
		{"/proc/self/fd/0", "/dev/stdin"},
		{"/proc/self/fd/1", "/dev/stdout"},
		{"/proc/self/fd/2", "/dev/stderr"},
	}

	for _, link := range symlinks {
		os.Remove(link.newname)
		if err := os.Symlink(link.oldname, link.newname); err != nil {
			fmt.Printf("[rootfs] Warning: symlink %s failed: %v\n", link.newname, err)
		}
	}

	return nil
}

// SetupMinimal sets up a minimal rootfs without pivot_root
// Used when pivot_root is not available or for simpler setups
func SetupMinimal() error {
	// Just remount /proc for the new PID namespace
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %w", err)
	}
	return nil
}
