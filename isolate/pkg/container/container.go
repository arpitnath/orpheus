// Package container orchestrates the full container lifecycle
package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"agentscale/isolate/pkg/cgroups"
)

// Config holds container configuration
type Config struct {
	// Resource limits
	MemoryMB   int // Memory limit in MB
	CPUPercent int // CPU limit as percentage
	MaxPIDs    int // Max processes

	// Filesystem
	RootFS string // Path to root filesystem (optional, uses host if empty)

	// Execution
	Command []string // Command to run
	Env     []string // Environment variables
	WorkDir string   // Working directory inside container
}

// Container represents a running container
type Container struct {
	ID     string
	Config *Config
	cgroup *cgroups.CGroup
	pid    int
}

// New creates a new container with the given configuration
func New(id string, config *Config) *Container {
	return &Container{
		ID:     id,
		Config: config,
	}
}

// Run executes the container
// This is called from the parent process to spawn the container
func (c *Container) Run() error {
	// Create cgroup first (before spawning child)
	if err := c.setupCgroup(); err != nil {
		return fmt.Errorf("failed to setup cgroup: %w", err)
	}

	// Spawn child process with namespaces
	cmd := exec.Command("/proc/self/exe", append([]string{"child"}, c.Config.Command...)...)

	// Set up namespaces
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // New hostname
			syscall.CLONE_NEWPID | // New PID namespace
			syscall.CLONE_NEWNS | // New mount namespace
			syscall.CLONE_NEWNET | // New network namespace (isolated)
			syscall.CLONE_NEWIPC, // New IPC namespace
		Unshareflags: syscall.CLONE_NEWNS,
	}

	// Pass config via environment
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CONTAINER_ID=%s", c.ID),
		fmt.Sprintf("CONTAINER_MEMORY=%d", c.Config.MemoryMB),
		fmt.Sprintf("CONTAINER_CPU=%d", c.Config.CPUPercent),
		fmt.Sprintf("CONTAINER_PIDS=%d", c.Config.MaxPIDs),
		fmt.Sprintf("CONTAINER_ROOTFS=%s", c.Config.RootFS),
		fmt.Sprintf("CONTAINER_WORKDIR=%s", c.Config.WorkDir),
	)

	// Connect stdio
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		c.Cleanup()
		return fmt.Errorf("failed to start container: %w", err)
	}

	c.pid = cmd.Process.Pid

	// Add to cgroup
	if c.cgroup != nil {
		if err := c.cgroup.AddProcess(c.pid); err != nil {
			fmt.Printf("[container] Warning: failed to add to cgroup: %v\n", err)
		}
	}

	// Wait for completion
	err := cmd.Wait()

	// Cleanup
	c.Cleanup()

	return err
}

// setupCgroup creates and configures the cgroup
func (c *Container) setupCgroup() error {
	// Check if cgroups v2 is available
	if !cgroups.IsCgroupV2() {
		fmt.Println("[container] Warning: cgroups v2 not available, running without resource limits")
		return nil
	}

	cg, err := cgroups.New(c.ID)
	if err != nil {
		return err
	}

	config := &cgroups.Config{
		MemoryLimitMB: c.Config.MemoryMB,
		CPUPercent:    c.Config.CPUPercent,
		MaxPIDs:       c.Config.MaxPIDs,
	}

	if err := cg.Apply(config); err != nil {
		cg.Remove()
		return err
	}

	c.cgroup = cg
	return nil
}

// Cleanup releases all container resources
func (c *Container) Cleanup() {
	if c.cgroup != nil {
		if err := c.cgroup.Remove(); err != nil {
			fmt.Printf("[container] Warning: failed to remove cgroup: %v\n", err)
		}
	}
}

// RunChild is called inside the container namespace
// It sets up the container environment and executes the command
func RunChild(command []string) error {
	// Get config from environment
	rootfs := os.Getenv("CONTAINER_ROOTFS")
	workdir := os.Getenv("CONTAINER_WORKDIR")

	fmt.Printf("[container] Child process started (PID %d in namespace)\n", os.Getpid())

	// Set hostname
	if err := syscall.Sethostname([]byte("container")); err != nil {
		fmt.Printf("[container] Warning: failed to set hostname: %v\n", err)
	}

	// Setup filesystem
	if rootfs != "" {
		// Full pivot_root setup
		if err := setupRootFS(rootfs); err != nil {
			return fmt.Errorf("failed to setup rootfs: %w", err)
		}
	} else {
		// Minimal setup - just remount /proc
		if err := setupMinimalFS(); err != nil {
			return fmt.Errorf("failed to setup minimal fs: %w", err)
		}
	}

	// Change to working directory
	if workdir != "" {
		if err := os.Chdir(workdir); err != nil {
			fmt.Printf("[container] Warning: failed to chdir to %s: %v\n", workdir, err)
		}
	}

	// Execute the command
	if len(command) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Find the executable
	execPath, err := exec.LookPath(command[0])
	if err != nil {
		// Try with /bin/sh -c
		execPath = "/bin/sh"
		command = []string{"/bin/sh", "-c", command[0]}
	}

	// Replace current process with the command
	return syscall.Exec(execPath, command, os.Environ())
}

// setupRootFS performs full rootfs setup with pivot_root
func setupRootFS(rootfs string) error {
	// Make mount namespace private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root private: %w", err)
	}

	// Bind mount the new root
	if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %w", err)
	}

	// Create old_root directory
	oldRoot := filepath.Join(rootfs, ".old_root")
	if err := os.MkdirAll(oldRoot, 0700); err != nil {
		return fmt.Errorf("failed to create old_root: %w", err)
	}

	// Pivot root
	if err := syscall.PivotRoot(rootfs, oldRoot); err != nil {
		return fmt.Errorf("pivot_root failed: %w", err)
	}

	// Chdir to new root
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir: %w", err)
	}

	// Mount /proc
	if err := os.MkdirAll("/proc", 0555); err == nil {
		syscall.Mount("proc", "/proc", "proc", 0, "")
	}

	// Unmount old root
	if err := syscall.Unmount("/.old_root", syscall.MNT_DETACH); err != nil {
		fmt.Printf("[container] Warning: failed to unmount old root: %v\n", err)
	}
	os.RemoveAll("/.old_root")

	return nil
}

// setupMinimalFS performs minimal filesystem setup (no pivot_root)
func setupMinimalFS() error {
	// Make mount namespace private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		fmt.Printf("[container] Warning: failed to make root private: %v\n", err)
	}

	// Remount /proc for the new PID namespace
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		fmt.Printf("[container] Warning: failed to mount /proc: %v\n", err)
	}

	return nil
}
