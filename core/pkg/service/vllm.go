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

// VLLMServer manages vLLM as a host process on Linux with NVIDIA GPU
type VLLMServer struct {
	modelName  string
	endpoint   string
	mode       ServerMode
	cmd        *exec.Cmd
	state      ServerState
	mu         sync.RWMutex
	httpClient *http.Client
	gpuDevice  int
}

// NewVLLMServer creates a new vLLM server manager
func NewVLLMServer(modelName string, mode ServerMode, endpoint string) *VLLMServer {
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	return &VLLMServer{
		modelName:  modelName,
		endpoint:   endpoint,
		mode:       mode,
		state:      StateStopped,
		gpuDevice:  0,
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}
}

// Mode returns the server management mode
func (v *VLLMServer) Mode() ServerMode {
	return v.mode
}

// Start starts the vLLM server process (managed mode) or verifies it's reachable (external mode)
func (v *VLLMServer) Start(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.mode == ServerModeExternal {
		log.Printf("[vllm] External mode - verifying server at %s", v.endpoint)
		v.state = StateStarting

		if healthy, err := v.Health(ctx); !healthy {
			v.state = StateStopped
			if err != nil {
				return fmt.Errorf("external vLLM server not reachable at %s: %w", v.endpoint, err)
			}
			return fmt.Errorf("external vLLM server not reachable at %s", v.endpoint)
		}

		v.state = StateReady
		log.Printf("[vllm] External server verified at %s", v.endpoint)
		return nil
	}

	if v.isRunning() {
		log.Printf("[vllm] Server already running at %s", v.endpoint)
		v.state = StateReady
		return nil
	}

	v.state = StateStarting
	log.Printf("[vllm] Starting vLLM server...")

	startupCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	args := []string{
		"-m", "vllm.entrypoints.openai.api_server",
		"--model", v.modelName,
		"--port", "8000",
		"--host", "0.0.0.0",
	}

	v.cmd = exec.CommandContext(startupCtx, "python3", args...)
	v.cmd.Env = append(os.Environ(),
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", v.gpuDevice),
	)

	if err := v.cmd.Start(); err != nil {
		v.state = StateStopped
		return fmt.Errorf("start vllm: %w", err)
	}

	log.Printf("[vllm] vLLM server started (PID: %d)", v.cmd.Process.Pid)

	v.state = StateLoading
	if err := v.waitForReady(startupCtx); err != nil {
		if v.cmd.Process != nil {
			v.cmd.Process.Kill()
		}
		v.state = StateStopped
		return fmt.Errorf("wait for ready: %w", err)
	}

	v.state = StateReady
	log.Printf("[vllm] Server ready at %s", v.endpoint)

	return nil
}

// Stop stops the vLLM server (managed mode only)
func (v *VLLMServer) Stop(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.mode == ServerModeExternal {
		log.Printf("[vllm] External mode - not stopping external server")
		return nil
	}

	if v.cmd == nil || v.cmd.Process == nil {
		return nil
	}

	log.Printf("[vllm] Stopping server...")

	if err := v.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		v.cmd.Process.Kill()
	}

	v.cmd.Wait()

	v.state = StateStopped
	log.Printf("[vllm] Server stopped")

	return nil
}

// Health checks if vLLM is responding
func (v *VLLMServer) Health(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", v.endpoint+"/health", nil)
	if err != nil {
		return false, err
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, nil
	}

	var result struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("invalid vLLM response: %w", err)
	}

	return result.Status == "ready", nil
}

// Endpoint returns the vLLM HTTP endpoint
func (v *VLLMServer) Endpoint() string {
	return v.endpoint
}

// Status returns current server status
func (v *VLLMServer) Status() ServerStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()

	healthy, _ := v.Health(context.Background())

	return ServerStatus{
		State:    v.state,
		Endpoint: v.endpoint,
		Model:    v.modelName,
		Healthy:  healthy,
	}
}

// Restart restarts the server
func (v *VLLMServer) Restart(ctx context.Context) error {
	if err := v.Stop(ctx); err != nil {
		return err
	}

	time.Sleep(2 * time.Second)

	return v.Start(ctx)
}

// isRunning checks if vLLM process is running
func (v *VLLMServer) isRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	healthy, _ := v.Health(ctx)
	return healthy
}

// GetProcess returns the OS process handle
func (v *VLLMServer) GetProcess() *os.Process {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.cmd != nil {
		return v.cmd.Process
	}
	return nil
}

// GetCommand returns the exec.Cmd for monitoring
func (v *VLLMServer) GetCommand() *exec.Cmd {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.cmd
}

// waitForReady waits for vLLM to be ready
func (v *VLLMServer) waitForReady(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeout:
			return fmt.Errorf("timeout waiting for vllm to be ready")

		case <-ticker.C:
			if healthy, _ := v.Health(ctx); healthy {
				return nil
			}
		}
	}
}
