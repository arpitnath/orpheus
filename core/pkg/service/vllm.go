package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// VLLMServer manages vLLM as a host process on Linux with NVIDIA GPU
type VLLMServer struct {
	modelName  string
	endpoint   string
	port       int
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

	// Extract port from endpoint (default 8000)
	port := 8000
	if strings.Contains(endpoint, ":") {
		parts := strings.Split(endpoint, ":")
		if len(parts) >= 3 {
			// http://host:port format
			portStr := strings.TrimSuffix(parts[2], "/")
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}
	}

	return &VLLMServer{
		modelName:  modelName,
		endpoint:   endpoint,
		port:       port,
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

	// Clean up any orphan vLLM processes before starting
	if err := v.cleanupOrphanProcesses(); err != nil {
		log.Printf("[vllm] Warning: orphan cleanup failed: %v", err)
	}

	v.state = StateStarting
	log.Printf("[vllm] Starting vLLM server...")

	// Create a startup timeout context for waiting, but NOT for the command itself
	// Using context.Background() for the command ensures it won't be killed when startup completes
	startupCtx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	args := []string{
		"-m", "vllm.entrypoints.openai.api_server",
		"--model", v.modelName,
		"--port", "8000",
		"--host", "0.0.0.0",
		"--gpu-memory-utilization", "0.7", // Reserve only 70% to leave headroom for other processes
	}

	// IMPORTANT: Use context.Background() for the command, NOT startupCtx
	// startupCtx is for timing out the wait, not the command lifecycle
	// Using startupCtx would kill vLLM when the context is canceled after successful startup
	v.cmd = exec.Command("python3", args...)
	v.cmd.Env = append(os.Environ(),
		fmt.Sprintf("CUDA_VISIBLE_DEVICES=%d", v.gpuDevice),
	)

	// Capture stdout/stderr for debugging
	v.cmd.Stdout = os.Stdout
	v.cmd.Stderr = os.Stderr

	// Set process group so we can kill all child processes (vLLM spawns EngineCore children)
	v.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
		// Even without tracked process, check for orphans
		v.cleanupOrphanProcesses()
		return nil
	}

	log.Printf("[vllm] Stopping server (PID: %d)...", v.cmd.Process.Pid)

	// Step 1: Kill process tree (all descendants, not just process group)
	if err := v.killProcessTree(v.cmd.Process.Pid); err != nil {
		log.Printf("[vllm] Process tree kill failed: %v, trying process group", err)

		// Fallback: Kill entire process group (negative PID)
		pgid, err := syscall.Getpgid(v.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
			time.Sleep(500 * time.Millisecond)
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	// Wait for main process
	done := make(chan error, 1)
	go func() {
		_, err := v.cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Printf("[vllm] Wait timeout, force killing")
		v.cmd.Process.Kill()
	}

	// Step 2: Cleanup any remaining orphans
	v.cleanupOrphanProcesses()

	// Step 3: Verify GPU memory is released
	if err := v.verifyGPUMemoryReleased(10 * time.Second); err != nil {
		log.Printf("[vllm] Warning: GPU memory may not be fully released: %v", err)
	}

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

	// vLLM's health endpoint returns HTTP 200 when ready
	// Some versions return empty body, others return {"status": "ready"}
	// Just checking status code is sufficient
	return resp.StatusCode == 200, nil
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

// waitForReady waits for vLLM to be ready using two-phase approach:
// Phase 1: Wait for TCP port to accept connections (fast feedback)
// Phase 2: Wait for HTTP /health endpoint to return 200 (model loaded)
// vLLM can take 2-3 minutes to load models into GPU memory on first start
func (v *VLLMServer) waitForReady(ctx context.Context) error {
	startTime := time.Now()

	// Phase 1: Wait for port binding (indicates process is alive and listening)
	log.Printf("[vllm] Phase 1: Waiting for port %d to accept connections...", v.port)
	if err := v.waitForPortBinding(ctx, 180*time.Second); err != nil {
		return fmt.Errorf("port binding failed after %v: %w", time.Since(startTime), err)
	}
	log.Printf("[vllm] Phase 1 complete: Port %d accepting connections after %v", v.port, time.Since(startTime))

	// Phase 2: Wait for HTTP health endpoint (model fully loaded)
	log.Printf("[vllm] Phase 2: Waiting for HTTP /health endpoint...")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(180 * time.Second) // 3 minutes for model loading

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timeout:
			return fmt.Errorf("timeout waiting for vllm HTTP health after %v (port was bound)", time.Since(startTime))

		case <-ticker.C:
			if healthy, _ := v.Health(ctx); healthy {
				log.Printf("[vllm] Phase 2 complete: HTTP health OK after %v total", time.Since(startTime))
				return nil
			}
		}
	}
}

// waitForPortBinding waits for the TCP port to accept connections
// This is faster feedback than HTTP health - indicates vLLM process is alive
func (v *VLLMServer) waitForPortBinding(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	addr := fmt.Sprintf("localhost:%d", v.port)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for port %d", v.port)
			}

			// Try TCP connection
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}

			// Check if vLLM process is still running
			if v.cmd != nil && v.cmd.Process != nil {
				if err := v.cmd.Process.Signal(syscall.Signal(0)); err != nil {
					return fmt.Errorf("vLLM process died while waiting for port: %w", err)
				}
			}
		}
	}
}

// cleanupOrphanProcesses finds and kills any orphan vLLM processes
// This handles processes that escaped process group cleanup
func (v *VLLMServer) cleanupOrphanProcesses() error {
	pids := v.findAllVLLMProcesses()
	if len(pids) == 0 {
		return nil
	}

	log.Printf("[vllm] Found %d orphan vLLM processes: %v", len(pids), pids)

	for _, pid := range pids {
		// SIGTERM first
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			log.Printf("[vllm] SIGTERM to PID %d failed: %v", pid, err)
		}
	}

	// Wait briefly for graceful shutdown
	time.Sleep(2 * time.Second)

	// SIGKILL any survivors
	for _, pid := range pids {
		if err := syscall.Kill(pid, syscall.Signal(0)); err == nil {
			// Still alive, force kill
			log.Printf("[vllm] Force killing orphan PID %d", pid)
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}

	return nil
}

// findAllVLLMProcesses finds all vLLM-related processes using pgrep
func (v *VLLMServer) findAllVLLMProcesses() []int {
	var pids []int

	// Find processes matching vLLM patterns
	// EngineCore is the GPU-holding child process that often escapes cleanup
	patterns := []string{
		"vllm.entrypoints",
		"vllm.engine",
		"vllm.v1.engine",
		"EngineCore",
		"multiprocessing.spawn",
	}

	for _, pattern := range patterns {
		cmd := exec.Command("pgrep", "-f", pattern)
		output, err := cmd.Output()
		if err != nil {
			continue // pgrep returns error if no matches
		}

		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			if pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text())); err == nil {
				// Don't include our own tracked process
				if v.cmd != nil && v.cmd.Process != nil && pid == v.cmd.Process.Pid {
					continue
				}
				pids = append(pids, pid)
			}
		}
	}

	return pids
}

// killProcessTree kills a process and all its descendants
func (v *VLLMServer) killProcessTree(pid int) error {
	// Find all child processes recursively
	children := v.findChildProcesses(pid)

	// Kill children first (bottom-up)
	for i := len(children) - 1; i >= 0; i-- {
		childPID := children[i]
		log.Printf("[vllm] Killing child process %d", childPID)
		syscall.Kill(childPID, syscall.SIGTERM)
	}

	// Brief pause for graceful shutdown
	time.Sleep(500 * time.Millisecond)

	// SIGKILL any survivors
	for _, childPID := range children {
		if err := syscall.Kill(childPID, syscall.Signal(0)); err == nil {
			syscall.Kill(childPID, syscall.SIGKILL)
		}
	}

	// Kill parent
	syscall.Kill(pid, syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	syscall.Kill(pid, syscall.SIGKILL)

	return nil
}

// findChildProcesses recursively finds all child processes of a given PID
func (v *VLLMServer) findChildProcesses(parentPID int) []int {
	var allChildren []int

	cmd := exec.Command("pgrep", "-P", strconv.Itoa(parentPID))
	output, err := cmd.Output()
	if err != nil {
		return allChildren
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		if pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text())); err == nil {
			allChildren = append(allChildren, pid)
			// Recursively find grandchildren
			grandchildren := v.findChildProcesses(pid)
			allChildren = append(allChildren, grandchildren...)
		}
	}

	return allChildren
}

// verifyGPUMemoryReleased checks that GPU memory has been freed after stopping vLLM
func (v *VLLMServer) verifyGPUMemoryReleased(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for GPU memory release")
			}

			memoryMB, err := v.getGPUMemoryUsage()
			if err != nil {
				log.Printf("[vllm] nvidia-smi check failed: %v", err)
				continue
			}

			// Consider memory released if under 500MB (some baseline usage is normal)
			if memoryMB < 500 {
				log.Printf("[vllm] GPU memory released: %d MB", memoryMB)
				return nil
			}

			log.Printf("[vllm] GPU memory still in use: %d MB, waiting...", memoryMB)
		}
	}
}

// getGPUMemoryUsage returns current GPU memory usage in MB using nvidia-smi
func (v *VLLMServer) getGPUMemoryUsage() (int, error) {
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=memory.used",
		"--format=csv,noheader,nounits",
		"-i", strconv.Itoa(v.gpuDevice))

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("nvidia-smi failed: %w", err)
	}

	memStr := strings.TrimSpace(string(output))
	memMB, err := strconv.Atoi(memStr)
	if err != nil {
		return 0, fmt.Errorf("parse memory value '%s': %w", memStr, err)
	}

	return memMB, nil
}
