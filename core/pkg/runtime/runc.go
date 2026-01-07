// Package runtime provides container runtime implementations for agent execution.
package runtime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"sync"

	"orpheus/daemon/pkg/oci"
)

// RuncResult holds the raw output from runc execution
type RuncResult struct {
	Stdout   string
	Stderr   string
	Err      error
	ExitCode int
	OOMKill  bool // True if exit code 137 (OOM killed)
}

// RuncRuntime wraps the runc binary for OCI container execution
type RuncRuntime struct {
	// BinaryPath is the path to the runc binary
	BinaryPath string
}

// NewRunc creates a new runc runtime, searching for the binary in standard locations
func NewRunc() *RuncRuntime {
	paths := []string{"/usr/bin/runc", "/usr/local/bin/runc", "runc"}
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return &RuncRuntime{BinaryPath: p}
		}
	}
	// Fallback to "runc" - will fail if not in PATH
	return &RuncRuntime{BinaryPath: "runc"}
}

// NewRuncWithPath creates a runc runtime with a specific binary path
func NewRuncWithPath(path string) *RuncRuntime {
	return &RuncRuntime{BinaryPath: path}
}

// Available returns true if runc binary is available
func (r *RuncRuntime) Available() bool {
	_, err := exec.LookPath(r.BinaryPath)
	return err == nil
}

// isInterfaceNil checks if an interface has a nil underlying value.
// In Go, an interface with a typed nil pointer is not nil, so we need reflection.
func isInterfaceNil(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// Run executes an agent in a runc container.
//
// Parameters:
//   - ctx: Context for cancellation/timeout
//   - bundle: OCI bundle (from pkg/oci)
//   - input: JSON input to pass via stdin
//   - monitor: Activity monitor interface for idle timeout (can be nil)
//
// Returns:
//   - *RuncResult: Raw execution result
//   - error: If container creation/execution fails at system level
func (r *RuncRuntime) Run(ctx context.Context, bundle *oci.Bundle, input string, monitor ActivityMonitorReader) (*RuncResult, error) {
	// Build runc command: runc run --bundle <path> <containerID>
	cmd := exec.CommandContext(ctx, r.BinaryPath, "run", "--bundle", bundle.Path, bundle.ID)
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
		return nil, fmt.Errorf("start runc: %w", err)
	}

	// Capture output with optional activity monitoring
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Check if monitor is actually usable (not a typed nil interface)
	hasMonitor := !isInterfaceNil(monitor)

	go func() {
		defer wg.Done()
		if hasMonitor {
			io.Copy(&stdout, monitor.MonitorReader(stdoutPipe, nil))
		} else {
			io.Copy(&stdout, stdoutPipe)
		}
	}()

	go func() {
		defer wg.Done()
		if hasMonitor {
			io.Copy(&stderr, monitor.MonitorReader(stderrPipe, nil))
		} else {
			io.Copy(&stderr, stderrPipe)
		}
	}()

	// Wait for output capture to complete
	wg.Wait()
	runErr := cmd.Wait()

	// Always cleanup container (best effort, ignore errors)
	r.Delete(bundle.ID)

	// Determine exit code
	exitCode := 0
	oomKill := false
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		// Check for OOM kill (exit code 137 = 128 + 9 SIGKILL)
		if exitCode == 137 {
			oomKill = true
		}
	}

	return &RuncResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      runErr,
		ExitCode: exitCode,
		OOMKill:  oomKill,
	}, nil
}

// RunStreaming executes an agent in a runc container with real-time SSE output streaming.
// Emits chunk events for each line of stdout/stderr while also buffering for final result.
func (r *RuncRuntime) RunStreaming(
	ctx context.Context,
	bundle *oci.Bundle,
	input string,
	monitor ActivityMonitorReader,
	streamWriter StreamWriter,
) (*RuncResult, error) {
	// Build runc command
	cmd := exec.CommandContext(ctx, r.BinaryPath, "run", "--bundle", bundle.Path, bundle.ID)
	cmd.Stdin = strings.NewReader(input)

	// Set up pipes
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start container
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start runc: %w", err)
	}

	// Capture output with streaming
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	// Wrap pipes with activity monitor if provided
	var stdoutReader io.Reader = stdoutPipe
	var stderrReader io.Reader = stderrPipe
	if !isInterfaceNil(monitor) {
		stdoutReader = monitor.MonitorReader(stdoutPipe, nil)
		stderrReader = monitor.MonitorReader(stderrPipe, nil)
	}

	// Stream stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutReader)
		for scanner.Scan() {
			line := scanner.Text()

			// Emit chunk event
			if streamWriter != nil {
				streamWriter.WriteEvent(NewChunkEvent("stdout", line))
			}

			// Also buffer for final result
			stdout.WriteString(line + "\n")
		}
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrReader)
		for scanner.Scan() {
			line := scanner.Text()

			// Emit chunk event
			if streamWriter != nil {
				streamWriter.WriteEvent(NewChunkEvent("stderr", line))
			}

			// Also buffer for final result
			stderr.WriteString(line + "\n")
		}
	}()

	// Wait for output capture to complete
	wg.Wait()
	runErr := cmd.Wait()

	// Always cleanup container
	r.Delete(bundle.ID)

	// Determine exit code
	exitCode := 0
	oomKill := false
	if runErr != nil {
		exitCode = 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if exitCode == 137 {
			oomKill = true
		}
	}

	return &RuncResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      runErr,
		ExitCode: exitCode,
		OOMKill:  oomKill,
	}, nil
}

// Delete force-deletes a container by ID
func (r *RuncRuntime) Delete(containerID string) error {
	cmd := exec.Command(r.BinaryPath, "delete", "--force", containerID)
	return cmd.Run()
}

// List returns all container IDs managed by runc
func (r *RuncRuntime) List() ([]string, error) {
	cmd := exec.Command(r.BinaryPath, "list", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ids = append(ids, line)
		}
	}
	return ids, nil
}
