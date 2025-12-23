// Package proxy handles agent execution and output capture.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
// Uses platform-based runtime selection:
//   - Linux + UseIsolate + RootFSPath: Use runc container
//   - macOS + UseIsolate + RootFSPath: Use Docker container
//   - Default: Run directly without isolation
func RunAgent(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *Result {
	startTime := time.Now()

	// Activity monitor setup (if enabled)
	var monitor *ActivityMonitor
	useActivityTimeout := opts != nil && opts.IdleTimeout > 0
	if useActivityTimeout {
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
				if useActivityTimeout {
					return runWithRuncAndMonitor(ctx, cfg, opts, monitor, startTime)
				}
				return runWithRunc(ctx, cfg, opts, startTime)
			}
			log.Println("WARN: runc not found, running without isolation")
		}

	case "darwin":
		if useIsolation {
			if runtime.DockerAvailable() {
				log.Println("INFO: macOS - using Docker for isolation")
				if useActivityTimeout {
					return runWithDockerAndMonitor(ctx, cfg, opts, monitor, startTime)
				}
				return runWithDocker(ctx, cfg, opts, startTime)
			}
			log.Println("WARN: macOS - running without isolation (Docker not available)")
		}
	}

	// Default: direct execution
	if useActivityTimeout {
		return runDirectExecWithMonitor(ctx, cfg, entrypointPath, opts, monitor, startTime)
	}
	return runDirectExec(ctx, cfg, entrypointPath, opts, startTime)
}

// runWithRunc executes an agent in a runc container (Linux isolation).
// Uses UUID-based container IDs for crash safety.
func runWithRunc(ctx context.Context, cfg *config.AgentConfig, opts *ExecuteOptions, startTime time.Time) *Result {
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

	// Run with runc (no activity monitor)
	runc := runtime.NewRunc()
	runcResult, err := runc.Run(ctx, bundle, input, nil)
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

// runWithRuncAndMonitor executes an agent in a runc container with activity monitoring.
func runWithRuncAndMonitor(ctx context.Context, cfg *config.AgentConfig, opts *ExecuteOptions, monitor *ActivityMonitor, startTime time.Time) *Result {
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

	// Run with runc and activity monitor
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

// runWithDocker executes an agent in a Docker container (macOS isolation).
func runWithDocker(ctx context.Context, cfg *config.AgentConfig, opts *ExecuteOptions, startTime time.Time) *Result {
	input := "{}"
	if opts != nil && opts.Input != "" {
		input = opts.Input
	}

	docker := runtime.NewDocker()
	dockerResult, err := docker.Run(ctx, cfg, opts.RootFSPath, input, nil)
	if err != nil {
		return NewErrorResult(err.Error(), "", 1, time.Since(startTime))
	}

	duration := time.Since(startTime)
	return ProcessResult(dockerResult.Stdout, dockerResult.Stderr, dockerResult.Err, duration)
}

// runWithDockerAndMonitor executes an agent in a Docker container with activity monitoring.
func runWithDockerAndMonitor(ctx context.Context, cfg *config.AgentConfig, opts *ExecuteOptions, monitor *ActivityMonitor, startTime time.Time) *Result {
	input := "{}"
	if opts != nil && opts.Input != "" {
		input = opts.Input
	}

	docker := runtime.NewDocker()
	dockerResult, err := docker.Run(ctx, cfg, opts.RootFSPath, input, monitor)
	if err != nil {
		return NewErrorResult(err.Error(), "", 1, time.Since(startTime))
	}

	duration := time.Since(startTime)
	return ProcessResult(dockerResult.Stdout, dockerResult.Stderr, dockerResult.Err, duration)
}

// runDirectExec executes an agent directly without isolation.
// Used for macOS dev mode or Linux without runc.
func runDirectExec(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions, startTime time.Time) *Result {
	input := "{}"
	if opts != nil && opts.Input != "" {
		input = opts.Input
	}

	directResult := runtime.RunDirect(ctx, cfg, entrypointPath, input, nil)
	duration := time.Since(startTime)
	return ProcessResult(directResult.Stdout, directResult.Stderr, directResult.Err, duration)
}

// runDirectExecWithMonitor executes an agent directly with activity monitoring.
func runDirectExecWithMonitor(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions, monitor *ActivityMonitor, startTime time.Time) *Result {
	input := "{}"
	if opts != nil && opts.Input != "" {
		input = opts.Input
	}

	directResult := runtime.RunDirect(ctx, cfg, entrypointPath, input, monitor)
	duration := time.Since(startTime)
	return ProcessResult(directResult.Stdout, directResult.Stderr, directResult.Err, duration)
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
