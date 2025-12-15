// Package proxy handles agent execution and output capture.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
}

// RunAgent executes the agent with the given configuration and entry point
func RunAgent(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, opts *ExecuteOptions) *Result {
	startTime := time.Now()

	// Determine working directory
	workDir := cfg.AgentDir
	if opts != nil && opts.WorkDir != "" {
		workDir = opts.WorkDir
	}

	// Create command with context for timeout
	var cmd *exec.Cmd
	useIsolation := opts != nil && opts.UseIsolate && opts.IsolatePath != ""

	// On macOS, VM isolation doesn't have Python yet - fall back to direct execution
	if useIsolation && runtime.GOOS == "darwin" {
		fmt.Fprintf(os.Stderr, "[agentscale] Warning: VM isolation on macOS doesn't have Python yet, running without isolation\n")
		useIsolation = false
	}

	if useIsolation {
		// Run inside isolate container (Linux namespace isolation)
		// isolate run --memory=512 "python3 /path/to/_entrypoint.py"
		cmd = exec.CommandContext(ctx, opts.IsolatePath,
			"run",
			fmt.Sprintf("--memory=%d", cfg.Memory),
			fmt.Sprintf("python3 %s", entrypointPath))
	} else {
		// Run directly (no isolation)
		cmd = exec.CommandContext(ctx, "python3", entrypointPath)
	}
	cmd.Dir = workDir

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

	// Set environment variables
	if cfg.Env != nil {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}
	if opts != nil && opts.Env != nil {
		cmd.Env = append(cmd.Environ(), opts.Env...)
	}

	// Run the command
	err := cmd.Run()
	duration := time.Since(startTime)

	// Check for context cancellation (timeout)
	if ctx.Err() == context.DeadlineExceeded {
		return NewTimeoutResult(duration)
	}

	// Get output strings
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

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
