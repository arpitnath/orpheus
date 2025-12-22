// Package proxy handles agent execution and output capture.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"agentscale/pkg/config"
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

	// IsolatePath is the path to the isolate binary
	IsolatePath string

	// RootFSPath is the path to agent image for --rootfs flag
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

// RunAgent executes the agent with the given configuration and entry point
// Uses activity-based timeout if IdleTimeout is configured, otherwise uses simple execution
func RunAgent(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *Result {
	// Check if activity-based timeout is enabled
	useActivityTimeout := opts != nil && opts.IdleTimeout > 0

	if useActivityTimeout {
		return runAgentWithActivityMonitor(ctx, cfg, entrypointPath, opts)
	}
	return runAgentSimple(ctx, cfg, entrypointPath, opts)
}

// buildCommand creates the exec.Cmd for agent execution
func buildCommand(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *exec.Cmd {
	var cmd *exec.Cmd
	useIsolation := opts != nil && opts.UseIsolate && opts.IsolatePath != ""
	hasRootFS := opts != nil && opts.RootFSPath != ""

	if useIsolation {
		// Build isolate command with Agent-Native memory flags
		memoryTarget := cfg.Memory
		memoryLimit := cfg.MemoryLimit
		swapEnabled := true // Default to enabled

		// Override with opts if provided
		if opts.MemoryTarget > 0 {
			memoryTarget = opts.MemoryTarget
		}
		if opts.MemoryLimit > 0 {
			memoryLimit = opts.MemoryLimit
		}
		if opts.MemoryLimit > 0 || opts.MemoryTarget > 0 {
			// If either is explicitly set, use SwapEnabled from opts
			swapEnabled = opts.SwapEnabled
		}

		// Ensure limit >= target
		if memoryLimit < memoryTarget {
			memoryLimit = memoryTarget * 2
		}

		args := []string{
			"run",
			fmt.Sprintf("--memory=%d", memoryTarget),
			fmt.Sprintf("--memory-limit=%d", memoryLimit),
		}
		if swapEnabled {
			args = append(args, "--swap-enabled")
		} else {
			args = append(args, "--no-swap")
		}

		if hasRootFS {
			// Add --rootfs flag for true filesystem isolation
			args = append(args, fmt.Sprintf("--rootfs=%s", opts.RootFSPath))
			// Entrypoint path inside container/VM
			// On macOS: VirtioFS mounts rootfs at /workspace
			entrypointPath = "/workspace/agent/_entrypoint.py"
		}

		// Use absolute Python path when using rootfs (deployed image)
		pythonCmd := "python3"
		if hasRootFS {
			// Python is at /usr/local/bin/python3 in VM initrd
			pythonCmd = "/usr/local/bin/python3"
		}

		args = append(args, fmt.Sprintf("%s %s", pythonCmd, entrypointPath))
		cmd = exec.CommandContext(ctx, opts.IsolatePath, args...)

		// Set PYTHONPATH and PATH for packages and agent code
		if hasRootFS {
			cmd.Env = append(cmd.Environ(),
				"PATH=/usr/local/bin:/usr/bin:/bin",
				"PYTHONPATH=/workspace/packages:/workspace/agent",
				"PYTHONUNBUFFERED=1",
				"PYTHONDONTWRITEBYTECODE=1")
		}
	} else {
		// Run directly (no isolation)
		cmd = exec.CommandContext(ctx, "python3", entrypointPath)
	}

	// Determine working directory
	workDir := cfg.AgentDir
	if opts != nil && opts.WorkDir != "" {
		workDir = opts.WorkDir
	}
	cmd.Dir = workDir

	// Set environment variables
	if cfg.Env != nil {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}
	if opts != nil && opts.Env != nil {
		cmd.Env = append(cmd.Environ(), opts.Env...)
	}

	return cmd
}

// runAgentSimple executes without activity monitoring (original behavior)
func runAgentSimple(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *Result {
	startTime := time.Now()

	cmd := buildCommand(ctx, cfg, entrypointPath, opts)

	// Set up stdin
	if opts != nil && opts.Input != "" {
		cmd.Stdin = strings.NewReader(opts.Input)
	} else {
		cmd.Stdin = strings.NewReader("{}")
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	duration := time.Since(startTime)

	// Check for context cancellation (timeout)
	if ctx.Err() == context.DeadlineExceeded {
		return NewTimeoutResult(duration)
	}

	return processResult(stdout.String(), stderr.String(), err, duration)
}

// runAgentWithActivityMonitor executes with activity-based timeout (Agent-Native)
func runAgentWithActivityMonitor(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *Result {
	startTime := time.Now()

	// Create activity monitor
	monitor := NewActivityMonitor(opts.IdleTimeout, opts.MaxTimeout, opts.ActivityCheck)
	defer monitor.Stop()

	cmd := buildCommand(ctx, cfg, entrypointPath, opts)

	// Set up stdin
	if opts != nil && opts.Input != "" {
		cmd.Stdin = strings.NewReader(opts.Input)
	} else {
		cmd.Stdin = strings.NewReader("{}")
	}

	// Get pipes for stdout/stderr to enable activity monitoring
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return NewErrorResult(fmt.Sprintf("failed to create stdout pipe: %v", err), "", 1, time.Since(startTime))
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return NewErrorResult(fmt.Sprintf("failed to create stderr pipe: %v", err), "", 1, time.Since(startTime))
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return NewErrorResult(fmt.Sprintf("failed to start command: %v", err), "", 1, time.Since(startTime))
	}

	// Capture output with activity monitoring
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Monitor stdout
	go func() {
		defer wg.Done()
		io.Copy(&stdout, monitor.MonitorReader(stdoutPipe, nil))
	}()

	// Monitor stderr
	go func() {
		defer wg.Done()
		io.Copy(&stderr, monitor.MonitorReader(stderrPipe, nil))
	}()

	// Start activity watching
	timeoutChan := monitor.StartWatching()

	// Wait for completion in a goroutine
	doneChan := make(chan error, 1)
	go func() {
		wg.Wait()
		doneChan <- cmd.Wait()
	}()

	// Wait for completion, activity timeout, or context cancellation
	select {
	case err := <-doneChan:
		// Normal completion
		monitor.Stop()
		duration := time.Since(startTime)
		return processResult(stdout.String(), stderr.String(), err, duration)

	case reason := <-timeoutChan:
		// Activity-based timeout triggered
		cmd.Process.Kill()
		wg.Wait() // Wait for goroutines to finish
		duration := time.Since(startTime)

		if reason == "idle_timeout" {
			return NewIdleTimeoutResult(duration, monitor.GetIdleTime())
		}
		return NewMaxTimeoutResult(duration)

	case <-ctx.Done():
		// Context cancelled (external timeout from caller)
		cmd.Process.Kill()
		wg.Wait() // Wait for goroutines to finish
		return NewTimeoutResult(time.Since(startTime))
	}
}

// processResult handles the output from agent execution
func processResult(stdoutStr, stderrStr string, err error, duration time.Duration) *Result {
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
