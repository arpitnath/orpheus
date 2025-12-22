package runtime

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"

	"agentscale/pkg/config"
)

// DirectResult holds the raw output from direct execution
type DirectResult struct {
	Stdout   string
	Stderr   string
	Err      error
	ExitCode int
}

// ActivityMonitorReader is an interface for activity monitoring
type ActivityMonitorReader interface {
	MonitorReader(r io.Reader, output io.Writer) io.Reader
}

// RunDirect executes an agent without isolation (direct Python execution).
// Used for:
//   - macOS development mode (no runc available)
//   - Linux without runc installed
//   - Testing without container overhead
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - cfg: Agent configuration
//   - entrypointPath: Path to the agent entrypoint script
//   - input: JSON input to pass via stdin
//   - monitor: Activity monitor interface for idle timeout (can be nil)
//
// Returns:
//   - *DirectResult: Raw execution result (stdout, stderr, error)
func RunDirect(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, input string, monitor ActivityMonitorReader) *DirectResult {
	// Build command
	cmd := exec.CommandContext(ctx, "python3", entrypointPath)
	cmd.Dir = cfg.AgentDir
	cmd.Stdin = strings.NewReader(input)

	// Set environment variables from config
	cmd.Env = append(cmd.Environ(), cfg.Env...)

	// Set up pipes for stdout/stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return &DirectResult{Err: err, ExitCode: 1}
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return &DirectResult{Err: err, ExitCode: 1}
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return &DirectResult{Err: err, ExitCode: 1}
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

	// Wait for output capture and process completion
	wg.Wait()
	err = cmd.Wait()

	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return &DirectResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
		ExitCode: exitCode,
	}
}
