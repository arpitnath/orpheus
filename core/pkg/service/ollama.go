package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// OllamaServer manages Ollama as a host process (macOS)
type OllamaServer struct {
	modelName string
	endpoint  string
	cmd       *exec.Cmd
	state     ServerState
	mu        sync.RWMutex
}

// NewOllamaServer creates a new Ollama server manager
func NewOllamaServer(modelName string) *OllamaServer {
	return &OllamaServer{
		modelName: modelName,
		endpoint:  "http://localhost:11434",
		state:     StateStopped,
	}
}

// Start starts the Ollama server process
func (o *OllamaServer) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check if already running
	if o.isRunning() {
		log.Printf("[ollama] Server already running at %s", o.endpoint)
		o.state = StateReady
		return nil
	}

	o.state = StateStarting
	log.Printf("[ollama] Starting Ollama server...")

	// Start Ollama serve
	o.cmd = exec.Command("ollama", "serve")

	if err := o.cmd.Start(); err != nil {
		o.state = StateStopped
		return fmt.Errorf("start ollama: %w", err)
	}

	log.Printf("[ollama] Ollama server started (PID: %d)", o.cmd.Process.Pid)

	// Wait for server to be ready
	o.state = StateLoading
	if err := o.waitForReady(ctx); err != nil {
		o.Stop(ctx)
		return fmt.Errorf("wait for ready: %w", err)
	}

	o.state = StateReady
	log.Printf("[ollama] Server ready at %s", o.endpoint)

	return nil
}

// Stop stops the Ollama server
func (o *OllamaServer) Stop(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

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
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(o.endpoint + "/api/tags")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200, nil
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
