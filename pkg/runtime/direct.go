package runtime

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/proxy"
)

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
//   - monitor: Activity monitor for idle timeout (can be nil)
//
// Returns:
//   - *proxy.Result: Execution result with output/error
func RunDirect(ctx context.Context, cfg *config.AgentConfig, entrypointPath string, input string, monitor *proxy.ActivityMonitor) *proxy.Result {
	startTime := time.Now()

	// Build command
	cmd := exec.CommandContext(ctx, "python3", entrypointPath)
	cmd.Dir = cfg.AgentDir
	cmd.Stdin = strings.NewReader(input)

	// Set environment variables from config
	cmd.Env = append(cmd.Environ(), cfg.Env...)

	// Set up pipes for stdout/stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return proxy.NewErrorResult(err.Error(), "", 1, time.Since(startTime))
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return proxy.NewErrorResult(err.Error(), "", 1, time.Since(startTime))
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return proxy.NewErrorResult(err.Error(), "", 1, time.Since(startTime))
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
	duration := time.Since(startTime)

	return proxy.ProcessResult(stdout.String(), stderr.String(), err, duration)
}
