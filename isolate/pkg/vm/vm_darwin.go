//go:build darwin

package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Code-Hex/vz/v3"
)

// DarwinVM implements VM interface using Apple Virtualization.framework
type DarwinVM struct {
	config       *Config
	machine      *vz.VirtualMachine
	running      bool
	mu           sync.Mutex
	vsockPort    uint32
	vsockListener net.Listener
}

// DarwinManager implements Manager interface for macOS
type DarwinManager struct {
	resourceDir string
	vm          *DarwinVM
	mu          sync.Mutex
}

// NewManager creates a new VM manager for macOS
func NewManager() Manager {
	homeDir, _ := os.UserHomeDir()
	resourceDir := filepath.Join(homeDir, ".agentscale", "vm")
	return &DarwinManager{
		resourceDir: resourceDir,
	}
}

// IsAvailable checks if Apple Virtualization.framework is available
func (m *DarwinManager) IsAvailable() bool {
	// Check by trying to create a simple configuration
	_, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		if errors.Is(err, vz.ErrUnsupportedOSVersion) {
			return false
		}
	}
	return true
}

// EnsureResources ensures kernel and rootfs are downloaded
func (m *DarwinManager) EnsureResources() error {
	if err := os.MkdirAll(m.resourceDir, 0755); err != nil {
		return fmt.Errorf("failed to create resource directory: %w", err)
	}

	kernelPath := filepath.Join(m.resourceDir, "vmlinuz")
	initrdPath := filepath.Join(m.resourceDir, "initrd")

	// Check kernel
	if _, err := os.Stat(kernelPath); os.IsNotExist(err) {
		return fmt.Errorf("kernel not found at %s - run: ./scripts/setup-vm-resources.sh", kernelPath)
	}

	// Check initrd
	if _, err := os.Stat(initrdPath); os.IsNotExist(err) {
		return fmt.Errorf("initrd not found at %s - run: ./scripts/setup-vm-resources.sh", initrdPath)
	}

	return nil
}

// Create creates a new VM with the given configuration
func (m *DarwinManager) Create(config *Config) (VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.vm != nil && m.vm.IsRunning() {
		return nil, ErrAlreadyRunning
	}

	// Set default paths
	if config.KernelPath == "" {
		config.KernelPath = filepath.Join(m.resourceDir, "vmlinuz")
	}
	if config.InitrdPath == "" {
		config.InitrdPath = filepath.Join(m.resourceDir, "initrd")
	}
	if config.RootFSPath == "" {
		config.RootFSPath = filepath.Join(m.resourceDir, "rootfs")
	}

	vm := &DarwinVM{
		config:    config,
		vsockPort: 1024, // Default vsock port for command execution
	}

	m.vm = vm
	return vm, nil
}

// Get returns the current VM if running
func (m *DarwinManager) Get() (VM, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.vm == nil {
		return nil, ErrNotRunning
	}
	return m.vm, nil
}

// Start boots the VM
func (v *DarwinVM) Start() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running {
		return ErrAlreadyRunning
	}

	// Build kernel command line
	// - console=hvc0: Use virtio console
	// - rdinit=/init: Use our init script in initramfs
	// - quiet: Suppress kernel boot messages
	// - loglevel=0: Only show critical errors
	cmdLine := "console=hvc0 rdinit=/init quiet loglevel=0"

	// Create boot loader
	bootLoader, err := vz.NewLinuxBootLoader(
		v.config.KernelPath,
		vz.WithCommandLine(cmdLine),
		vz.WithInitrd(v.config.InitrdPath),
	)
	if err != nil {
		return fmt.Errorf("failed to create boot loader: %w", err)
	}

	// Create VM configuration
	vmConfig, err := vz.NewVirtualMachineConfiguration(
		bootLoader,
		uint(v.config.CPUs),
		uint64(v.config.MemoryMB*1024*1024),
	)
	if err != nil {
		return fmt.Errorf("failed to create VM config: %w", err)
	}

	// Add serial console (for seeing boot output)
	if err := v.addSerialConsole(vmConfig); err != nil {
		return fmt.Errorf("failed to add serial console: %w", err)
	}

	// Add entropy device (required for Linux)
	if err := v.addEntropyDevice(vmConfig); err != nil {
		return fmt.Errorf("failed to add entropy device: %w", err)
	}

	// Add memory balloon
	if err := v.addMemoryBalloon(vmConfig); err != nil {
		return fmt.Errorf("failed to add memory balloon: %w", err)
	}

	// Add VirtioFS for shared directory (if configured)
	if v.config.SharedDir != "" {
		if err := v.addVirtioFS(vmConfig); err != nil {
			fmt.Printf("[vm] Warning: Failed to add VirtioFS: %v\n", err)
			// Continue without VirtioFS
		}
	}

	// Add vsock for command execution
	if err := v.addVsock(vmConfig); err != nil {
		fmt.Printf("[vm] Warning: Failed to add vsock: %v\n", err)
		// Continue without vsock
	}

	// Validate configuration
	validated, err := vmConfig.Validate()
	if err != nil {
		return fmt.Errorf("invalid VM config: %w", err)
	}
	if !validated {
		return fmt.Errorf("VM config validation failed")
	}

	// Create VM
	v.machine, err = vz.NewVirtualMachine(vmConfig)
	if err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	// Start VM in a goroutine and wait for it to be running
	errCh := make(chan error, 1)
	go func() {
		errCh <- v.machine.Start()
	}()

	// Wait for start or timeout
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("failed to start VM: %w", err)
		}
	case <-time.After(30 * time.Second):
		return fmt.Errorf("VM start timed out")
	}

	v.running = true
	fmt.Println("[vm] VM started successfully")

	return nil
}

func (v *DarwinVM) addSerialConsole(vmConfig *vz.VirtualMachineConfiguration) error {
	// Create a PTY for the serial console
	serialPortAttachment, err := vz.NewFileHandleSerialPortAttachment(
		os.Stdin,
		os.Stdout,
	)
	if err != nil {
		return err
	}

	consoleConfig, err := vz.NewVirtioConsoleDeviceSerialPortConfiguration(serialPortAttachment)
	if err != nil {
		return err
	}

	vmConfig.SetSerialPortsVirtualMachineConfiguration([]*vz.VirtioConsoleDeviceSerialPortConfiguration{
		consoleConfig,
	})

	return nil
}

func (v *DarwinVM) addEntropyDevice(vmConfig *vz.VirtualMachineConfiguration) error {
	entropyConfig, err := vz.NewVirtioEntropyDeviceConfiguration()
	if err != nil {
		return err
	}

	vmConfig.SetEntropyDevicesVirtualMachineConfiguration([]*vz.VirtioEntropyDeviceConfiguration{
		entropyConfig,
	})

	return nil
}

func (v *DarwinVM) addMemoryBalloon(vmConfig *vz.VirtualMachineConfiguration) error {
	balloonConfig, err := vz.NewVirtioTraditionalMemoryBalloonDeviceConfiguration()
	if err != nil {
		return err
	}

	vmConfig.SetMemoryBalloonDevicesVirtualMachineConfiguration([]vz.MemoryBalloonDeviceConfiguration{
		balloonConfig,
	})

	return nil
}

func (v *DarwinVM) addVirtioFS(vmConfig *vz.VirtualMachineConfiguration) error {
	// Create shared directory
	sharedDir, err := vz.NewSharedDirectory(v.config.SharedDir, false) // false = not read-only
	if err != nil {
		return fmt.Errorf("failed to create shared directory: %w", err)
	}

	// Create directory share
	shareConfig, err := vz.NewSingleDirectoryShare(sharedDir)
	if err != nil {
		return fmt.Errorf("failed to create directory share: %w", err)
	}

	// Create VirtioFS device
	fsConfig, err := vz.NewVirtioFileSystemDeviceConfiguration("workspace")
	if err != nil {
		return fmt.Errorf("failed to create VirtioFS config: %w", err)
	}
	fsConfig.SetDirectoryShare(shareConfig)

	vmConfig.SetDirectorySharingDevicesVirtualMachineConfiguration([]vz.DirectorySharingDeviceConfiguration{
		fsConfig,
	})

	return nil
}

func (v *DarwinVM) addVsock(vmConfig *vz.VirtualMachineConfiguration) error {
	// Create vsock device for host-guest communication
	vsockConfig, err := vz.NewVirtioSocketDeviceConfiguration()
	if err != nil {
		return fmt.Errorf("failed to create vsock config: %w", err)
	}

	vmConfig.SetSocketDevicesVirtualMachineConfiguration([]vz.SocketDeviceConfiguration{
		vsockConfig,
	})

	return nil
}

// Stop shuts down the VM
func (v *DarwinVM) Stop() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.running {
		return ErrNotRunning
	}

	if v.vsockListener != nil {
		v.vsockListener.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try graceful stop first
	if v.machine.CanRequestStop() {
		stopped, err := v.machine.RequestStop()
		if err == nil && stopped {
			v.running = false
			fmt.Println("[vm] VM stopped gracefully")
			return nil
		}
	}

	// Force stop
	if v.machine.CanStop() {
		err := v.machine.Stop()
		if err != nil {
			// Try waiting a bit
			select {
			case <-ctx.Done():
				return fmt.Errorf("failed to stop VM: timeout")
			case <-time.After(time.Second):
			}
		}
	}

	v.running = false
	fmt.Println("[vm] VM stopped")

	return nil
}

// IsRunning returns true if VM is running
func (v *DarwinVM) IsRunning() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.running
}

// Run executes a command inside the VM
func (v *DarwinVM) Run(command string) (string, error) {
	if !v.IsRunning() {
		return "", ErrNotRunning
	}

	// For now, commands are sent via serial console
	// A more robust implementation would use vsock
	return "", fmt.Errorf("Run not fully implemented - VM runs in interactive mode")
}

// RunInteractive executes a command with stdin/stdout/stderr attached
func (v *DarwinVM) RunInteractive(command string, stdin io.Reader, stdout, stderr io.Writer) error {
	if !v.IsRunning() {
		return ErrNotRunning
	}

	// The VM is already connected to stdin/stdout via serial console
	// Commands can be typed directly
	return fmt.Errorf("VM is running in interactive mode - type commands directly")
}

// WaitForBoot waits for the VM to boot and be ready
func (v *DarwinVM) WaitForBoot(timeout time.Duration) error {
	// Wait for VM state to be running
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		state := v.machine.State()
		if state == vz.VirtualMachineStateRunning {
			// Give the kernel a moment to initialize
			time.Sleep(2 * time.Second)
			return nil
		}
		if state == vz.VirtualMachineStateError {
			return fmt.Errorf("VM entered error state")
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("VM boot timed out")
}
