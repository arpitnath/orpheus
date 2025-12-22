package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"agentscale/pkg/config"
)

// DockerResult holds the raw output from Docker execution
type DockerResult struct {
	Stdout   string
	Stderr   string
	Err      error
	ExitCode int
}

// DockerRuntime wraps Docker for container execution on macOS
type DockerRuntime struct{}

// NewDocker creates a new Docker runtime
func NewDocker() *DockerRuntime {
	return &DockerRuntime{}
}

// Run executes an agent in a Docker container.
// Used for macOS isolation when Docker Desktop is available.
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - cfg: Agent configuration
//   - imagePath: Path to agent image directory (mounted as /workspace)
//   - input: JSON input to pass via stdin
//   - monitor: Activity monitor interface for idle timeout (can be nil)
//
// Returns:
//   - *DockerResult: Raw execution result
//   - error: If container creation fails at system level
func (d *DockerRuntime) Run(ctx context.Context, cfg *config.AgentConfig, imagePath string, input string, monitor ActivityMonitorReader) (*DockerResult, error) {
	// Build docker run command with security flags matching runc config
	args := []string{
		"run", "--rm", "-i",
		// Mount agent image as /workspace (read-only)
		"-v", imagePath + ":/workspace:ro",
		"-w", "/workspace/agent",
		// Memory limit (same as OCI config)
		"--memory", fmt.Sprintf("%dm", cfg.MemoryLimit),
		// PID limit (prevent fork bombs)
		"--pids-limit", "256",
		// Non-root user (same as OCI config: uid 1000)
		"--user", "1000:1000",
		// Drop all capabilities (same as OCI config)
		"--cap-drop", "ALL",
		// Environment variables
		"-e", "PYTHONPATH=/workspace/packages:/workspace/agent",
		"-e", "PYTHONUNBUFFERED=1",
		"-e", "PYTHONDONTWRITEBYTECODE=1",
		"-e", "HOME=/tmp",
	}

	// Add agent-specific environment variables
	for _, env := range cfg.Env {
		args = append(args, "-e", env)
	}

	// Base image and command
	args = append(args,
		"python:3.10-slim",
		"python", "/workspace/agent/_entrypoint.py",
	)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(input)

	// Set up pipes for stdout/stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start the container
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start docker: %w", err)
	}

	// Capture output with optional activity monitoring
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if monitor != nil {
			io.Copy(&stdout, monitor.MonitorReader(stdoutPipe, nil))
		} else {
			io.Copy(&stdout, stdoutPipe)
		}
	}()

	go func() {
		defer wg.Done()
		if monitor != nil {
			io.Copy(&stderr, monitor.MonitorReader(stderrPipe, nil))
		} else {
			io.Copy(&stderr, stderrPipe)
		}
	}()

	// Wait for output capture to complete
	wg.Wait()
	runErr := cmd.Wait()

	// Determine exit code
	exitCode := 0
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return &DockerResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      runErr,
		ExitCode: exitCode,
	}, nil
}
