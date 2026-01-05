// Package proxy handles agent execution and output capture.
package proxy

import "time"

// Status represents the execution status of an agent
type Status string

const (
	StatusSuccess     Status = "success"
	StatusError       Status = "error"
	StatusTimeout     Status = "timeout"      // Max timeout exceeded
	StatusIdleTimeout Status = "idle_timeout" // No activity timeout (Agent-Native)
)

// Result represents the output of an agent execution
type Result struct {
	// Status of the execution
	Status Status `json:"status"`

	// Output contains the parsed JSON output from the agent
	Output map[string]interface{} `json:"output,omitempty"`

	// RawOutput contains the raw stdout from the agent
	RawOutput string `json:"raw_output,omitempty"`

	// Error message if status is error or timeout
	Error string `json:"error,omitempty"`

	// Stderr output from the agent
	Stderr string `json:"stderr,omitempty"`

	// ExitCode from the process
	ExitCode int `json:"exit_code"`

	// Duration of execution
	Duration time.Duration `json:"duration"`
}

// NewSuccessResult creates a successful result
func NewSuccessResult(output map[string]interface{}, duration time.Duration) *Result {
	return &Result{
		Status:   StatusSuccess,
		Output:   output,
		ExitCode: 0,
		Duration: duration,
	}
}

// NewErrorResult creates an error result
func NewErrorResult(err string, stderr string, exitCode int, duration time.Duration) *Result {
	return &Result{
		Status:   StatusError,
		Error:    err,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: duration,
	}
}

// NewTimeoutResult creates a timeout result (max timeout exceeded)
func NewTimeoutResult(duration time.Duration) *Result {
	return &Result{
		Status:   StatusTimeout,
		Error:    "execution timed out (max timeout exceeded)",
		ExitCode: -1,
		Duration: duration,
	}
}

// NewIdleTimeoutResult creates an idle timeout result (Agent-Native: no activity)
func NewIdleTimeoutResult(totalDuration, idleDuration time.Duration) *Result {
	return &Result{
		Status:   StatusIdleTimeout,
		Error:    "agent idle for " + idleDuration.String() + " (no activity detected)",
		ExitCode: -1,
		Duration: totalDuration,
	}
}

// NewMaxTimeoutResult creates a max timeout result with explicit message
func NewMaxTimeoutResult(duration time.Duration) *Result {
	return &Result{
		Status:   StatusTimeout,
		Error:    "execution exceeded maximum timeout (" + duration.String() + ")",
		ExitCode: -1,
		Duration: duration,
	}
}
