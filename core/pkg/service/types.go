package service

import (
	"context"
	"os"
	"os/exec"
)

// ServerState represents the current state of a model server
type ServerState string

const (
	StateStopped   ServerState = "stopped"
	StateStarting  ServerState = "starting"
	StateLoading   ServerState = "loading"
	StateReady     ServerState = "ready"
	StateServing   ServerState = "serving"
	StateUnhealthy ServerState = "unhealthy"
)

// ServerStatus contains current status information
type ServerStatus struct {
	State    ServerState
	Endpoint string
	Model    string
	Healthy  bool
	Uptime   int64 // seconds
}

// ModelServer defines the interface for managing model servers
// Platform-specific implementations (Ollama on Mac, vLLM on Linux)
type ModelServer interface {
	// Start initializes and starts the model server
	Start(ctx context.Context) error

	// Stop gracefully shuts down the model server
	Stop(ctx context.Context) error

	// Health checks if the server is responding
	Health(ctx context.Context) (bool, error)

	// Endpoint returns the HTTP endpoint for inference
	Endpoint() string

	// Status returns current server status
	Status() ServerStatus

	// Restart stops and starts the server
	Restart(ctx context.Context) error

	// GetProcess returns the OS process handle (nil for container-based backends)
	GetProcess() *os.Process

	// GetCommand returns exec.Cmd for process monitoring (nil for container-based)
	GetCommand() *exec.Cmd
}

// ModelConfig specifies model server configuration
type ModelConfig struct {
	Name     string            // Model name (e.g., "mistral-7b")
	Server   string            // Server type: "auto", "ollama", "vllm", "external"
	BaseURL  string            // For external servers
	Options  map[string]string // Server-specific options
}

type HealthEventType int

const (
	HealthCheckFailed HealthEventType = iota
	HealthCheckPassed
)

type HealthEvent struct {
	Type     HealthEventType
	Failures int
}
