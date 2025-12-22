//go:build !linux

package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"agentscale/isolate/pkg/vm"
)

// Global VM manager for reuse across invocations
var vmManager vm.Manager

// createNamespacedProcess runs the command inside a VM on non-Linux platforms
func createNamespacedProcess(config *Config) error {
	// On macOS, use VM-based isolation
	if runtime.GOOS == "darwin" {
		return runInVM(config)
	}

	// On other non-Linux platforms, warn and fail
	return fmt.Errorf("isolation not supported on %s", runtime.GOOS)
}

// runInVM executes the command inside the macOS VM
func runInVM(config *Config) error {
	// Initialize VM manager if needed
	if vmManager == nil {
		vmManager = vm.NewManager()
	}

	// Ensure VM resources are available
	if err := vmManager.EnsureResources(); err != nil {
		return fmt.Errorf("VM resources not available: %w\nRun: ./scripts/setup-vm-resources.sh && ./scripts/build-initrd.sh", err)
	}

	// Try to get existing VM, or create new one
	vmInstance, err := vmManager.Get()
	if err == vm.ErrNotRunning {
		// Create and start VM
		memMB := uint64(config.MemoryMB)
		if memMB < 256 {
			memMB = 256 // Minimum for VM
		}
		vmConfig := &vm.Config{
			CPUs:      2,
			MemoryMB:  memMB,
			SharedDir: config.RootFS, // Mount rootfs via VirtioFS
		}

		vmInstance, err = vmManager.Create(vmConfig)
		if err != nil {
			return fmt.Errorf("failed to create VM: %w", err)
		}

		fmt.Println("[isolate] Starting VM...")
		if err := vmInstance.Start(); err != nil {
			return fmt.Errorf("failed to start VM: %w", err)
		}

		// Wait for boot
		fmt.Println("[isolate] Waiting for VM to boot...")
		if err := vmInstance.WaitForBoot(30 * time.Second); err != nil {
			return fmt.Errorf("VM boot failed: %w", err)
		}

		// Give vsock-agent time to start
		time.Sleep(time.Second)
		fmt.Println("[isolate] VM ready")
	} else if err != nil {
		return fmt.Errorf("failed to get VM: %w", err)
	}

	// Read stdin if available
	stdinData := ""
	if stat, _ := os.Stdin.Stat(); (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil {
			stdinData = string(data)
		}
	}

	// Execute command in VM
	// Python is now in initrd at /usr/local/bin/python3 (no PATH prefix needed)
	fmt.Printf("[isolate] Executing in VM: %s\n", config.Command)

	// Use the VM interface - need to cast to access RunWithStdin
	darwinVM, ok := vmInstance.(*vm.DarwinVM)
	if !ok {
		return fmt.Errorf("unexpected VM type")
	}

	var output string
	if stdinData != "" {
		output, err = darwinVM.RunWithStdin(config.Command, stdinData)
	} else {
		output, err = darwinVM.Run(config.Command)
	}

	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}

	// Print output
	fmt.Print(output)

	return nil
}

// runInsideNamespace - stub for non-Linux platforms
func runInsideNamespace() {
	fmt.Fprintf(os.Stderr, "[child] Error: namespace isolation not available on %s\n", runtime.GOOS)
	os.Exit(1)
}
