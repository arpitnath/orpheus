// agentscale-runtime is the Go runtime binary for executing agents.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/runner"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	// Define subcommands
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("agentscale-runtime %s\n", Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`agentscale-runtime - Agent execution runtime

Usage:
  agentscale-runtime run <agent-dir> [options]
  agentscale-runtime version
  agentscale-runtime help

Commands:
  run       Execute an agent
  version   Print version information
  help      Show this help message

Run Options:
  --memory <mb>     Override memory limit (default: from agent.yaml or 512)
  --timeout <sec>   Override timeout in seconds (default: from agent.yaml or 60)
  --no-isolate      Skip container isolation (run directly)
  --async           Use async template for entry point
  --keep-entrypoint Keep generated _entrypoint.py after execution

Examples:
  echo '{"query": "hello"}' | agentscale-runtime run ./my-agent --no-isolate
  agentscale-runtime run ./my-agent --timeout 120 --no-isolate`)
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)

	// Define flags
	memory := fs.Int("memory", 0, "Override memory limit in MB")
	timeout := fs.Int("timeout", 0, "Override timeout in seconds")
	noIsolate := fs.Bool("no-isolate", false, "Skip container isolation")
	async := fs.Bool("async", false, "Use async template")
	keepEntrypoint := fs.Bool("keep-entrypoint", false, "Keep generated _entrypoint.py")

	// Parse flags
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Get agent directory
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: agent directory required")
		fmt.Fprintln(os.Stderr, "Usage: agentscale-runtime run <agent-dir> [options]")
		os.Exit(1)
	}
	agentDir := fs.Arg(0)

	// Load configuration
	cfg, err := config.Load(agentDir)
	if err != nil {
		outputError(err)
		os.Exit(1)
	}

	// Apply overrides
	if *memory > 0 {
		cfg.Memory = *memory
	}
	if *timeout > 0 {
		cfg.Timeout = time.Duration(*timeout) * time.Second
	}

	// Create runner
	r := runner.New(cfg)

	// Set up options
	opts := &runner.RunOptions{
		Async:          *async,
		NoIsolate:      *noIsolate,
		KeepEntrypoint: *keepEntrypoint,
	}

	// Execute
	ctx := context.Background()
	result, err := r.Run(ctx, opts)
	if err != nil {
		outputError(err)
		os.Exit(1)
	}

	// Output result
	fmt.Println(runner.OutputJSON(result))

	// Exit with appropriate code
	if result.Status != "success" {
		os.Exit(1)
	}
}

func outputError(err error) {
	fmt.Printf(`{"status": "error", "error": %q}`+"\n", err.Error())
}
