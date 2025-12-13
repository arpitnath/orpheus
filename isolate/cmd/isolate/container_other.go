//go:build !linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// createNamespacedProcess - stub for non-Linux platforms
func createNamespacedProcess(config *Config) error {
	fmt.Printf("[isolate] WARNING: Namespace isolation not available on %s\n", runtime.GOOS)
	fmt.Printf("[isolate] Running command without isolation (development mode)\n")

	// On non-Linux, just run the command directly without isolation
	cmd := exec.Command("/bin/sh", "-c", config.Command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// runInsideNamespace - stub for non-Linux platforms
func runInsideNamespace() {
	fmt.Fprintf(os.Stderr, "[child] Error: namespace isolation not available on %s\n", runtime.GOOS)
	os.Exit(1)
}
