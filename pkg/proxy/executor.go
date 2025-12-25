// Package proxy handles agent execution and output capture.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"agentscale/pkg/config"
	"agentscale/pkg/oci"
	"agentscale/pkg/runtime"
)

// ExecuteOptions holds options for agent execution
type ExecuteOptions struct {
	// Input is the JSON input to pass to stdin
	Input string

	// Env contains additional environment variables
	Env []string

	// WorkDir overrides the working directory
	WorkDir string

	// UseIsolate enables container isolation
	UseIsolate bool

	// RootFSPath is the path to deployed agent image directory
	RootFSPath string

	// Memory configuration (Agent-Native: Graceful Degradation)
	MemoryTarget int  // Target memory in MB (soft limit - fast tier)
	MemoryLimit  int  // Hard limit in MB (with swap)
	SwapEnabled  bool // Enable swap for graceful degradation

	// Timeout configuration (Agent-Native: Activity-Based)
	IdleTimeout   time.Duration // No activity timeout
	MaxTimeout    time.Duration // Absolute maximum timeout
	ActivityCheck time.Duration // Activity check interval
}

// RunAgent executes the agent with the given configuration and entry point.
// Platform-based runtime selection:
//   - Linux + UseIsolate + RootFSPath: Use runc container
//   - macOS: Requires Lima VM (Phase 7+) - returns error if isolation requested
//   - No isolation: Returns error (unsafe execution removed in Phase 6)
func RunAgent(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *Result {
	startTime := time.Now()

	// Activity monitor setup (if enabled)
	var monitor *ActivityMonitor
	if opts != nil && opts.IdleTimeout > 0 {
		monitor = NewActivityMonitor(opts.IdleTimeout, opts.MaxTimeout, opts.ActivityCheck)
		defer monitor.Stop()
	}

	// Check if isolation is requested with a deployed agent image
	useIsolation := opts != nil && opts.UseIsolate && opts.RootFSPath != ""

	// Platform-based runtime selection
	switch goruntime.GOOS {
	case "linux":
		if useIsolation {
			if runtime.RuncAvailable() {
				return runWithRunc(ctx, cfg, opts, monitor, startTime)
			}
			return NewErrorResult("runc not available: install runc for container isolation", "", 1, time.Since(startTime))
		}
		// No isolation requested - this is an error (removed unsafe direct execution)
		return NewErrorResult("isolation required: use --isolate with a deployed agent image", "", 1, time.Since(startTime))

	case "darwin":
		// macOS cannot run containers natively.
		//
		// Architecture Note:
		//   Normal operation: CLI → Lima VM → daemon (Linux) → runc
		//   In daemon mode, this path should NEVER execute because
		//   the daemon runs as a Linux binary inside Lima VM.
		//
		// This error only occurs if:
		//   1. User hasn't started Lima VM ('agentscale vm start')
		//   2. Direct (non-daemon) call to proxy.RunAgent() on macOS
		//
		// Solution: Ensure daemon is running in Lima VM.
		return NewErrorResult(
			"macOS requires Lima VM for agent execution. "+
				"Start VM with: agentscale vm start",
			"", 1, time.Since(startTime))

	default:
		return NewErrorResult(fmt.Sprintf("unsupported platform: %s", goruntime.GOOS), "", 1, time.Since(startTime))
	}
}

// runWithRunc executes an agent in a runc container (Linux isolation).
// Consolidated function that handles both monitored and non-monitored execution.
func runWithRunc(ctx context.Context, cfg *config.AgentConfig, opts *ExecuteOptions, monitor *ActivityMonitor, startTime time.Time) *Result {
	// Generate UUID-based container ID (prevents collisions)
	containerID := fmt.Sprintf("agent-%s", uuid.New().String())

	// Generate OCI bundle
	bundleGen := oci.NewBundleGenerator()
	bundle, err := bundleGen.GenerateBundle(cfg, opts.RootFSPath, containerID)
	if err != nil {
		return NewErrorResult(fmt.Sprintf("bundle generation: %v", err), "", 1, time.Since(startTime))
	}
	defer bundle.Cleanup()

	// Get input
	input := "{}"
	if opts != nil && opts.Input != "" {
		input = opts.Input
	}

	// Run with runc (monitor can be nil - runc.Run handles it)
	runc := runtime.NewRunc()
	runcResult, err := runc.Run(ctx, bundle, input, monitor)
	if err != nil {
		return NewErrorResult(err.Error(), "", 1, time.Since(startTime))
	}

	// Convert runc result to proxy result
	duration := time.Since(startTime)
	if runcResult.OOMKill {
		return NewErrorResult("OOM killed: agent exceeded memory limit", runcResult.Stderr, runcResult.ExitCode, duration)
	}
	return ProcessResult(runcResult.Stdout, runcResult.Stderr, runcResult.Err, duration)
}

// ProcessResult handles the output from agent execution.
// Exported for use by runtime package.
func ProcessResult(stdoutStr, stderrStr string, err error, duration time.Duration) *Result {
	stdoutStr = strings.TrimSpace(stdoutStr)
	stderrStr = strings.TrimSpace(stderrStr)

	// Handle execution error
	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		// Try to parse error from stdout (our entrypoint writes errors as JSON)
		if stdoutStr != "" {
			var output map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(stdoutStr), &output); jsonErr == nil {
				if errMsg, ok := output["error"].(string); ok {
					return &Result{
						Status:    StatusError,
						Output:    output,
						Error:     errMsg,
						Stderr:    stderrStr,
						RawOutput: stdoutStr,
						ExitCode:  exitCode,
						Duration:  duration,
					}
				}
			}
		}

		return NewErrorResult(err.Error(), stderrStr, exitCode, duration)
	}

	// Parse JSON output
	if stdoutStr == "" {
		return &Result{
			Status:   StatusSuccess,
			Output:   map[string]interface{}{},
			ExitCode: 0,
			Duration: duration,
		}
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(stdoutStr), &output); err != nil {
		// Not valid JSON, return raw output
		return &Result{
			Status:    StatusSuccess,
			RawOutput: stdoutStr,
			Output:    map[string]interface{}{"raw": stdoutStr},
			ExitCode:  0,
			Duration:  duration,
		}
	}

	// Check if output contains error status
	if status, ok := output["status"].(string); ok && status == "error" {
		errMsg := "unknown error"
		if e, ok := output["error"].(string); ok {
			errMsg = e
		}
		return &Result{
			Status:    StatusError,
			Output:    output,
			Error:     errMsg,
			RawOutput: stdoutStr,
			ExitCode:  0, // Process succeeded but agent returned error
			Duration:  duration,
		}
	}

	return &Result{
		Status:    StatusSuccess,
		Output:    output,
		RawOutput: stdoutStr,
		ExitCode:  0,
		Duration:  duration,
	}
}
