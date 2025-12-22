package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the container configuration (Agent-Native)
type Config struct {
	Command string

	// Memory configuration (Agent-Native: Graceful Degradation)
	MemoryMB      int  // Target memory in MB (soft limit - fast tier)
	MemoryLimitMB int  // Hard limit in MB (with swap)
	SwapEnabled   bool // Enable swap for graceful degradation

	// Other limits
	CPUPercent int    // CPU limit as percentage (50 = 50%)
	MaxPIDs    int    // Max number of processes
	RootFS     string // Path to root filesystem (optional)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runContainer()
	case "child":
		// Internal: called after namespace setup
		runChild()
	case "vm":
		handleVMCommand()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`isolate - Lightweight container runtime (Agent-Native)

Usage:
  isolate run [options] <command>    Run a command in isolation
  isolate vm <subcommand>            Manage VM (macOS only)
  isolate help                       Show this help

Run Options:
  --memory=<MB>        Memory target in MB - fast tier (default: 256)
  --memory-limit=<MB>  Memory hard limit in MB - with swap (default: 512)
  --swap-enabled       Enable swap for graceful degradation (default: true)
  --no-swap            Disable swap (hard OOM kill at memory target)
  --cpu=<percent>      CPU limit as percentage (default: 100)
  --pids=<max>         Max processes (default: 100)
  --rootfs=<path>      Path to root filesystem (enables pivot_root isolation)

Agent-Native Memory Behavior:
  0 to target (256MB):     Fast performance (pure RAM)
  target to limit (512MB): Slower (swapping to disk, but survives)
  above limit:             OOM kill (hard limit exceeded)

VM Subcommands (macOS only):
  isolate vm status                  Check VM status
  isolate vm start                   Start the VM
  isolate vm stop                    Stop the VM
  isolate vm setup                   Setup VM resources (kernel, rootfs)

Examples:
  isolate run "echo hello"
  isolate run --memory=256 --memory-limit=512 "python script.py"
  isolate run --rootfs=/path/to/rootfs "ls -la /"
  isolate run --memory=128 --no-swap "lightweight_agent.py"`)
}

func parseArgs(args []string) (*Config, error) {
	config := &Config{
		// Agent-Native memory defaults
		MemoryMB:      256,  // Default 256MB target (soft limit)
		MemoryLimitMB: 512,  // Default 512MB hard limit
		SwapEnabled:   true, // Enable swap by default

		// Other defaults
		CPUPercent: 100, // Default 100% (no limit)
		MaxPIDs:    100, // Default 100 processes
	}

	var commandParts []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if strings.HasPrefix(arg, "--memory=") {
			val := strings.TrimPrefix(arg, "--memory=")
			mem, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid memory value: %s", val)
			}
			config.MemoryMB = mem
		} else if strings.HasPrefix(arg, "--memory-limit=") {
			val := strings.TrimPrefix(arg, "--memory-limit=")
			mem, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid memory-limit value: %s", val)
			}
			config.MemoryLimitMB = mem
		} else if arg == "--swap-enabled" {
			config.SwapEnabled = true
		} else if arg == "--no-swap" {
			config.SwapEnabled = false
		} else if strings.HasPrefix(arg, "--cpu=") {
			val := strings.TrimPrefix(arg, "--cpu=")
			cpu, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid cpu value: %s", val)
			}
			config.CPUPercent = cpu
		} else if strings.HasPrefix(arg, "--pids=") {
			val := strings.TrimPrefix(arg, "--pids=")
			pids, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid pids value: %s", val)
			}
			config.MaxPIDs = pids
		} else if strings.HasPrefix(arg, "--rootfs=") {
			config.RootFS = strings.TrimPrefix(arg, "--rootfs=")
		} else if !strings.HasPrefix(arg, "-") {
			// Everything else is the command
			commandParts = append(commandParts, arg)
		} else {
			return nil, fmt.Errorf("unknown option: %s", arg)
		}
	}

	if len(commandParts) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	// Ensure memory limit >= memory target
	if config.MemoryLimitMB < config.MemoryMB {
		config.MemoryLimitMB = config.MemoryMB * 2
	}

	config.Command = strings.Join(commandParts, " ")
	return config, nil
}

func runContainer() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Error: no command specified")
		printUsage()
		os.Exit(1)
	}

	config, err := parseArgs(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[isolate] Starting container (Agent-Native)...\n")
	fmt.Printf("[isolate] Memory: target=%dMB (fast), limit=%dMB (hard), swap=%v\n",
		config.MemoryMB, config.MemoryLimitMB, config.SwapEnabled)
	fmt.Printf("[isolate] Limits: cpu=%d%%, pids=%d\n",
		config.CPUPercent, config.MaxPIDs)
	if config.RootFS != "" {
		fmt.Printf("[isolate] RootFS: %s\n", config.RootFS)
	}
	fmt.Printf("[isolate] Command: %s\n", config.Command)

	// Platform-specific: create namespaced process
	if err := createNamespacedProcess(config); err != nil {
		fmt.Fprintf(os.Stderr, "[isolate] Container failed: %v\n", err)
		os.Exit(1)
	}
}

func runChild() {
	// Platform-specific: run inside namespace
	runInsideNamespace()
}
