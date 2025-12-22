//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"agentscale/isolate/pkg/cgroups"
)

// Global cgroup reference for cleanup
var containerCgroup *cgroups.CGroup

// createNamespacedProcess creates a new process with isolated namespaces (Linux only)
func createNamespacedProcess(config *Config) error {
	// Generate container ID
	containerID := fmt.Sprintf("isolate-%d", os.Getpid())

	// Setup cgroup before spawning child (Agent-Native: Graceful Degradation)
	if cgroups.IsCgroupV2() {
		cg, err := cgroups.New(containerID)
		if err != nil {
			fmt.Printf("[isolate] Warning: failed to create cgroup: %v\n", err)
		} else {
			cgConfig := &cgroups.Config{
				// Agent-Native memory config
				MemoryTargetMB: config.MemoryMB,      // Soft limit (fast tier)
				MemoryLimitMB:  config.MemoryLimitMB, // Hard limit (with swap)
				SwapEnabled:    config.SwapEnabled,   // Enable swap for graceful degradation

				// Other limits
				CPUPercent: config.CPUPercent,
				MaxPIDs:    config.MaxPIDs,
			}
			if err := cg.Apply(cgConfig); err != nil {
				fmt.Printf("[isolate] Warning: failed to apply cgroup limits: %v\n", err)
			}
			containerCgroup = cg
		}
	} else {
		fmt.Println("[isolate] Warning: cgroups v2 not available, running without resource limits")
	}

	// Re-exec ourselves with "child" subcommand inside new namespaces
	cmd := exec.Command("/proc/self/exe", "child", config.Command,
		fmt.Sprintf("%d", config.MemoryMB),
		fmt.Sprintf("%d", config.CPUPercent),
		fmt.Sprintf("%d", config.MaxPIDs),
		config.RootFS,
	)

	// Set up namespaces using clone flags
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // New hostname namespace
			syscall.CLONE_NEWPID | // New PID namespace
			syscall.CLONE_NEWNS | // New mount namespace
			syscall.CLONE_NEWIPC, // New IPC namespace
		Unshareflags: syscall.CLONE_NEWNS,
	}

	// Connect stdin/stdout/stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		cleanup()
		return err
	}

	// Add child to cgroup
	if containerCgroup != nil {
		if err := containerCgroup.AddProcess(cmd.Process.Pid); err != nil {
			fmt.Printf("[isolate] Warning: failed to add process to cgroup: %v\n", err)
		}
	}

	// Wait for completion
	err := cmd.Wait()

	// Cleanup
	cleanup()

	return err
}

// cleanup removes cgroup and other resources
func cleanup() {
	if containerCgroup != nil {
		if err := containerCgroup.Remove(); err != nil {
			fmt.Printf("[isolate] Warning: failed to remove cgroup: %v\n", err)
		}
		containerCgroup = nil
	}
}

// runInsideNamespace is called after we're inside the new namespace
func runInsideNamespace() {
	// Args: child <command> <memoryMB> <cpuPercent> <maxPIDs> <rootfs>
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "[child] Error: missing command")
		os.Exit(1)
	}

	command := os.Args[2]

	// Parse resource limits from args (for informational purposes)
	memoryMB := 0
	cpuPercent := 0
	maxPIDs := 0
	rootfs := ""
	if len(os.Args) >= 4 {
		memoryMB, _ = strconv.Atoi(os.Args[3])
	}
	if len(os.Args) >= 5 {
		cpuPercent, _ = strconv.Atoi(os.Args[4])
	}
	if len(os.Args) >= 6 {
		maxPIDs, _ = strconv.Atoi(os.Args[5])
	}
	if len(os.Args) >= 7 {
		rootfs = os.Args[6]
	}

	fmt.Printf("[container] PID: %d (namespace PID 1)\n", os.Getpid())
	fmt.Printf("[container] Limits: memory=%dMB, cpu=%d%%, pids=%d\n", memoryMB, cpuPercent, maxPIDs)

	// Set hostname for this namespace
	if err := syscall.Sethostname([]byte("container")); err != nil {
		fmt.Fprintf(os.Stderr, "[container] Warning: failed to set hostname: %v\n", err)
	}

	// Setup filesystem
	if rootfs != "" {
		fmt.Printf("[container] Setting up rootfs: %s\n", rootfs)
		if err := setupPivotRoot(rootfs); err != nil {
			fmt.Fprintf(os.Stderr, "[container] Failed to setup rootfs: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Minimal setup - just remount /proc
		if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
			fmt.Fprintf(os.Stderr, "[container] Warning: failed to make root private: %v\n", err)
		}
		if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
			fmt.Fprintf(os.Stderr, "[container] Warning: failed to mount /proc: %v\n", err)
		}
	}

	fmt.Printf("[container] Executing: %s\n", command)

	// Execute the command using /bin/sh -c
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "[container] Command failed: %v\n", err)
		os.Exit(1)
	}
}

// setupPivotRoot sets up filesystem isolation using pivot_root
func setupPivotRoot(rootfs string) error {
	// Make mount namespace private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root private: %w", err)
	}

	// Bind mount the new root
	if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %w", err)
	}

	// Create old_root directory
	oldRoot := rootfs + "/.old_root"
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

	// Mount /proc for the new PID namespace
	if err := os.MkdirAll("/proc", 0555); err == nil {
		if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
			fmt.Printf("[container] Warning: failed to mount /proc: %v\n", err)
		}
	}

	// Unmount old root
	if err := syscall.Unmount("/.old_root", syscall.MNT_DETACH); err != nil {
		fmt.Printf("[container] Warning: failed to unmount old root: %v\n", err)
	}

	// Remove old root directory
	os.RemoveAll("/.old_root")

	return nil
}
