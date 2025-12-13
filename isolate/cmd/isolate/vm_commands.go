package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"agentscale/isolate/pkg/vm"
)

func handleVMCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: isolate vm <subcommand>")
		fmt.Println("")
		fmt.Println("Subcommands:")
		fmt.Println("  status    Check VM status")
		fmt.Println("  start     Start the VM")
		fmt.Println("  stop      Stop the VM")
		fmt.Println("  setup     Setup VM resources")
		os.Exit(1)
	}

	manager := vm.NewManager()

	// Check if VM is available on this platform
	if !manager.IsAvailable() && os.Args[2] != "status" {
		if runtime.GOOS != "darwin" {
			fmt.Printf("[vm] VM not needed on %s - namespace isolation is native\n", runtime.GOOS)
		} else {
			fmt.Println("[vm] Apple Virtualization.framework not available")
			fmt.Println("[vm] Requires macOS 11.0 (Big Sur) or later")
		}
		os.Exit(1)
	}

	switch os.Args[2] {
	case "status":
		vmStatus(manager)
	case "start":
		vmStart(manager)
	case "stop":
		vmStop(manager)
	case "setup":
		vmSetup(manager)
	default:
		fmt.Fprintf(os.Stderr, "Unknown VM subcommand: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func vmStatus(manager vm.Manager) {
	fmt.Printf("[vm] Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	if runtime.GOOS != "darwin" {
		fmt.Println("[vm] VM not needed - Linux namespace isolation is native")
		return
	}

	if !manager.IsAvailable() {
		fmt.Println("[vm] Status: Apple Virtualization.framework NOT available")
		fmt.Println("[vm] Requires macOS 11.0 (Big Sur) or later")
		return
	}

	fmt.Println("[vm] Status: Apple Virtualization.framework available")

	// Check resources
	if err := manager.EnsureResources(); err != nil {
		fmt.Printf("[vm] Resources: NOT ready - %v\n", err)
	} else {
		fmt.Println("[vm] Resources: Ready")
	}

	// Check if VM is running
	existingVM, err := manager.Get()
	if err == nil && existingVM.IsRunning() {
		fmt.Println("[vm] VM: Running")
	} else {
		fmt.Println("[vm] VM: Not running")
	}
}

func vmStart(manager vm.Manager) {
	fmt.Println("[vm] Checking resources...")

	if err := manager.EnsureResources(); err != nil {
		fmt.Fprintf(os.Stderr, "[vm] Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[vm] Creating VM...")

	config := vm.DefaultConfig()
	vmInstance, err := manager.Create(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vm] Failed to create VM: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[vm] Starting VM...")
	fmt.Println("[vm] Press Ctrl+C to stop the VM")
	fmt.Println("")

	if err := vmInstance.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[vm] Failed to start VM: %v\n", err)
		os.Exit(1)
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for VM to stop or signal
	<-sigCh

	fmt.Println("")
	fmt.Println("[vm] Stopping VM...")
	vmInstance.Stop()
	fmt.Println("[vm] VM stopped")
}

func vmStop(manager vm.Manager) {
	vmInstance, err := manager.Get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[vm] No running VM found: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[vm] Stopping VM...")

	if err := vmInstance.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "[vm] Failed to stop VM: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[vm] VM stopped successfully")
}

func vmSetup(manager vm.Manager) {
	fmt.Println("[vm] Setting up VM resources...")

	if err := manager.EnsureResources(); err != nil {
		fmt.Printf("[vm] Setup incomplete: %v\n", err)
		fmt.Println("")
		fmt.Println("To complete setup, you need to provide a Linux kernel and initrd.")
		fmt.Println("See ~/.agentscale/vm/README.md for instructions.")
		os.Exit(1)
	}

	fmt.Println("[vm] Resources are ready!")
}
