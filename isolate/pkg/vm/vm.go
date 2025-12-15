package vm

import (
	"fmt"
	"io"
	"time"
)

// Config holds VM configuration
type Config struct {
	CPUs      uint   // Number of CPUs
	MemoryMB  uint64 // Memory in MB
	KernelPath string // Path to Linux kernel
	InitrdPath string // Path to initrd (optional)
	RootFSPath string // Path to root filesystem
	SharedDir  string // Directory to share with VM
}

// DefaultConfig returns sensible defaults for the VM
func DefaultConfig() *Config {
	return &Config{
		CPUs:     2,
		MemoryMB: 512,
	}
}

// VM represents a virtual machine instance
type VM interface {
	// Start boots the VM
	Start() error

	// Stop shuts down the VM
	Stop() error

	// IsRunning returns true if VM is running
	IsRunning() bool

	// Run executes a command inside the VM and returns output
	Run(command string) (string, error)

	// RunInteractive executes a command with stdin/stdout/stderr attached
	RunInteractive(command string, stdin io.Reader, stdout, stderr io.Writer) error

	// WaitForBoot waits for the VM to boot and be ready
	WaitForBoot(timeout time.Duration) error
}

// Manager handles VM lifecycle
type Manager interface {
	// Create creates a new VM with the given config
	Create(config *Config) (VM, error)

	// Get returns an existing VM if running
	Get() (VM, error)

	// IsAvailable returns true if VM support is available on this platform
	IsAvailable() bool

	// EnsureResources ensures kernel/rootfs are available
	EnsureResources() error
}

// ErrNotAvailable is returned when VM support is not available
var ErrNotAvailable = fmt.Errorf("VM support not available on this platform")

// ErrNotRunning is returned when VM is not running
var ErrNotRunning = fmt.Errorf("VM is not running")

// ErrAlreadyRunning is returned when VM is already running
var ErrAlreadyRunning = fmt.Errorf("VM is already running")
