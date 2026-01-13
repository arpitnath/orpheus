package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// OllamaServer manages Ollama as a host process (macOS) or external server
type OllamaServer struct {
	modelName  string
	endpoint   string
	mode       ServerMode // managed or external
	cmd        *exec.Cmd
	state      ServerState
	mu         sync.RWMutex
	httpClient *http.Client
}

// NewOllamaServer creates a new Ollama server manager
// mode: ServerModeManaged (process management) or ServerModeExternal (health checks only)
// endpoint: URL of the Ollama server (e.g., "http://localhost:11434" or "http://host.lima.internal:11434")
func NewOllamaServer(modelName string, mode ServerMode, endpoint string) *OllamaServer {
	// Default endpoint based on mode
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}

	return &OllamaServer{
		modelName:  modelName,
		endpoint:   endpoint,
		mode:       mode,
		state:      StateStopped,
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}
}

// Mode returns the server management mode
func (o *OllamaServer) Mode() ServerMode {
	return o.mode
}

// Start starts the Ollama server process (managed mode) or verifies it's reachable (external mode)
func (o *OllamaServer) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// External mode: Just verify server is reachable, don't try to start process
	if o.mode == ServerModeExternal {
		log.Printf("[ollama] External mode - verifying server at %s", o.endpoint)
		o.state = StateStarting

		if healthy, err := o.Health(ctx); !healthy {
			o.state = StateStopped
			if err != nil {
				return fmt.Errorf("external Ollama server not reachable at %s: %w", o.endpoint, err)
			}
			return fmt.Errorf("external Ollama server not reachable at %s", o.endpoint)
		}

		o.state = StateReady
		log.Printf("[ollama] External server verified at %s", o.endpoint)
		return nil
	}

	// Managed mode: Start the process if not already running
	if o.isRunning() {
		log.Printf("[ollama] Server already running at %s", o.endpoint)
		o.state = StateReady
		return nil
	}

	o.state = StateStarting
	log.Printf("[ollama] Starting Ollama server...")

	startupCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	o.cmd = exec.CommandContext(startupCtx, "ollama", "serve")
	o.cmd.Env = append(os.Environ(),
		"OLLAMA_NUM_PARALLEL=4",
		"OLLAMA_MAX_LOADED_MODELS=2",
	)

	if err := o.cmd.Start(); err != nil {
		o.state = StateStopped
		return fmt.Errorf("start ollama: %w", err)
	}

	log.Printf("[ollama] Ollama server started (PID: %d)", o.cmd.Process.Pid)

	o.state = StateLoading
	if err := o.waitForReady(startupCtx); err != nil {
		if o.cmd.Process != nil {
			o.cmd.Process.Kill()
		}
		o.state = StateStopped
		return fmt.Errorf("wait for ready: %w", err)
	}

	o.state = StateReady
	log.Printf("[ollama] Server ready at %s", o.endpoint)

	return nil
}

// Stop stops the Ollama server (managed mode only - external mode does nothing)
func (o *OllamaServer) Stop(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// External mode: Don't stop external servers
	if o.mode == ServerModeExternal {
		log.Printf("[ollama] External mode - not stopping external server")
		return nil
	}

	// Managed mode: Stop the process
	if o.cmd == nil || o.cmd.Process == nil {
		return nil
	}

	log.Printf("[ollama] Stopping server...")

	// Try graceful shutdown first
	if err := o.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Force kill if graceful fails
		o.cmd.Process.Kill()
	}

	// Wait for process to exit
	o.cmd.Wait()

	o.state = StateStopped
	log.Printf("[ollama] Server stopped")

	return nil
}

// Health checks if Ollama is responding
func (o *OllamaServer) Health(ctx context.Context) (bool, error) {
	resp, err := o.httpClient.Get(o.endpoint + "/api/tags")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, nil
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("invalid Ollama response: %w", err)
	}

	return true, nil
}

// Endpoint returns the Ollama HTTP endpoint
func (o *OllamaServer) Endpoint() string {
	return o.endpoint
}

// Status returns current server status
func (o *OllamaServer) Status() ServerStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()

	healthy, _ := o.Health(context.Background())

	return ServerStatus{
		State:    o.state,
		Endpoint: o.endpoint,
		Model:    o.modelName,
		Healthy:  healthy,
	}
}

// Restart restarts the server
func (o *OllamaServer) Restart(ctx context.Context) error {
	if err := o.Stop(ctx); err != nil {
		return err
	}

	time.Sleep(2 * time.Second) // Brief pause

	return o.Start(ctx)
}

// isRunning checks if Ollama process is running
func (o *OllamaServer) isRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	healthy, _ := o.Health(ctx)
	return healthy
}

// GetProcess returns the OS process handle
func (o *OllamaServer) GetProcess() *os.Process {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.cmd != nil {
		return o.cmd.Process
	}
	return nil
}

// GetCommand returns the exec.Cmd for monitoring
func (o *OllamaServer) GetCommand() *exec.Cmd {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return o.cmd
}

// waitForReady waits for Ollama to be ready
func (o *OllamaServer) waitForReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeout:
			return fmt.Errorf("timeout waiting for ollama to be ready")

		case <-ticker.C:
			if healthy, _ := o.Health(ctx); healthy {
				return nil
			}
		}
	}
}
