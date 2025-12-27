package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"agentscale/pkg/registry"
	"agentscale/pkg/scaling"

	"gopkg.in/yaml.v3"
)

// AgentPool holds all components for one agent's autoscaling.
type AgentPool struct {
	agentName string
	agentPath string
	queue     *scaling.RequestQueue
	pool      *scaling.BasicWorkerPool
	policy    scaling.ScalingPolicy

	// Per-agent lifecycle
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWg     sync.WaitGroup
}

// PoolManager manages autoscaling pools for all deployed agents.
// It creates pools on-demand when agents are deployed via the registry.
type PoolManager struct {
	registry   registry.Registry
	autoscaler *scaling.BasicAutoscaler

	pools map[string]*AgentPool // agentName → pool
	mu    sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPoolManager creates a new pool manager.
func NewPoolManager(registry registry.Registry, autoscaler *scaling.BasicAutoscaler, ctx context.Context) *PoolManager {
	ctx, cancel := context.WithCancel(ctx)
	return &PoolManager{
		registry:   registry,
		autoscaler: autoscaler,
		pools:      make(map[string]*AgentPool),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// CreatePool creates an autoscaling pool for the given agent.
// This is called when an agent is deployed.
func (pm *PoolManager) CreatePool(agentName string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already exists
	if _, exists := pm.pools[agentName]; exists {
		return fmt.Errorf("pool already exists for %s", agentName)
	}

	// Get agent from registry
	agent, err := pm.registry.Get(agentName)
	if err != nil {
		return fmt.Errorf("agent not found in registry: %w", err)
	}

	// Load scaling config from agent.yaml (or use defaults)
	policy, queueSize := pm.loadScalingConfig(agent.Path)

	// Create queue
	queue := scaling.NewRequestQueue(agentName, queueSize)

	// Create spawner
	spawner := NewDaemonWorkerSpawner(agentName, agent.Path, agent.ResolvedEnv)

	// Create pool
	pool := scaling.NewWorkerPool(agentName, spawner, policy)

	// Create per-agent context
	workerCtx, workerCancel := context.WithCancel(pm.ctx)

	agentPool := &AgentPool{
		agentName:    agentName,
		agentPath:    agent.Path,
		queue:        queue,
		pool:         pool,
		policy:       policy,
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}

	// Register with autoscaler
	pm.autoscaler.RegisterPool(agentName, pool, policy)
	pm.autoscaler.RegisterQueueMetrics(agentName, queue)

	// Start worker loops (initial workers = minWorkers)
	numWorkers := policy.MinWorkers
	if numWorkers < 1 {
		numWorkers = 1
	}

	for i := 0; i < numWorkers; i++ {
		pm.wg.Add(1)
		go pm.workerLoop(agentPool)
	}

	pm.pools[agentName] = agentPool
	log.Printf("[pool-manager] Created pool for '%s' (min=%d, max=%d, queue=%d)",
		agentName, policy.MinWorkers, policy.MaxWorkers, queueSize)

	return nil
}

// GetPool retrieves an existing pool for the given agent.
func (pm *PoolManager) GetPool(agentName string) (*AgentPool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pool, ok := pm.pools[agentName]
	if !ok {
		return nil, fmt.Errorf("no pool for agent: %s", agentName)
	}
	return pool, nil
}

// RemovePool removes an autoscaling pool for the given agent.
// This is called when an agent is undeployed.
func (pm *PoolManager) RemovePool(agentName string) error {
	pm.mu.Lock()
	agentPool, exists := pm.pools[agentName]
	if !exists {
		pm.mu.Unlock()
		return fmt.Errorf("no pool for agent: %s", agentName)
	}
	delete(pm.pools, agentName)
	pm.mu.Unlock()

	log.Printf("[pool-manager] Removing pool for '%s'", agentName)

	// Unregister from autoscaler
	pm.autoscaler.UnregisterPool(agentName)

	// Close queue (stops accepting new requests)
	agentPool.queue.Close()

	// Cancel worker loops
	agentPool.workerCancel()

	// Wait for workers to finish current tasks
	agentPool.workerWg.Wait()

	// Shutdown pool (drains workers gracefully)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := agentPool.pool.Shutdown(shutdownCtx); err != nil {
		log.Printf("[pool-manager] Pool shutdown error for %s: %v", agentName, err)
	}

	log.Printf("[pool-manager] Removed pool for '%s'", agentName)
	return nil
}

// workerLoop processes requests from an agent pool's queue.
// This loop runs in the background for each agent.
func (pm *PoolManager) workerLoop(agentPool *AgentPool) {
	defer pm.wg.Done()

	for {
		// Dequeue request from THIS agent's queue
		req, err := agentPool.queue.Dequeue(agentPool.workerCtx)
		if err != nil {
			// Context cancelled - shutdown
			return
		}

		// Get worker from THIS agent's pool
		worker, err := agentPool.pool.GetIdleWorker(req.Context)
		if err != nil {
			// Failed to get worker - send error response
			req.ResponseCh <- &scaling.Response{
				Error:    err,
				Duration: 0,
			}
			agentPool.queue.Complete(req.ID)
			continue
		}

		// Check if streaming requested (Phase 3)
		if req.StreamCh != nil {
			// Streaming execution
			pm.executeStreaming(req, worker, agentPool)
		} else {
			// Non-streaming execution
			pm.executeNonStreaming(req, worker, agentPool)
		}
	}
}

// executeStreaming handles streaming execution via worker pool.
func (pm *PoolManager) executeStreaming(req *scaling.Request, worker scaling.Worker, agentPool *AgentPool) {
	// Type-assert to DaemonWorker for streaming support
	daemonWorker, ok := worker.(*DaemonWorker)
	if !ok {
		// Worker doesn't support streaming - fall back to non-streaming
		pm.executeNonStreaming(req, worker, agentPool)
		return
	}

	start := time.Now()

	// Execute with streaming
	result, err := daemonWorker.ExecuteStreaming(req.Context, req.Input, req.StreamCh)
	duration := time.Since(start)

	// Return worker to pool
	agentPool.pool.ReturnWorker(worker)

	// Send final response
	req.ResponseCh <- &scaling.Response{
		Result:   result,
		Error:    err,
		Duration: duration,
	}

	// Close stream channel (signals completion to handler)
	if req.StreamCh != nil {
		close(req.StreamCh)
	}

	// Mark request complete
	agentPool.queue.Complete(req.ID)
}

// executeNonStreaming handles non-streaming execution via worker pool.
func (pm *PoolManager) executeNonStreaming(req *scaling.Request, worker scaling.Worker, agentPool *AgentPool) {
	start := time.Now()
	result, err := worker.Execute(req.Context, req.Input)
	duration := time.Since(start)

	// Return worker to pool
	agentPool.pool.ReturnWorker(worker)

	// Send response
	req.ResponseCh <- &scaling.Response{
		Result:   result,
		Error:    err,
		Duration: duration,
	}

	// Mark request complete
	agentPool.queue.Complete(req.ID)
}

// Shutdown gracefully shuts down all pools.
func (pm *PoolManager) Shutdown(ctx context.Context) error {
	log.Printf("[pool-manager] Shutting down all pools...")

	// Cancel context to stop worker loops
	pm.cancel()

	// Wait for all worker loops to finish
	pm.wg.Wait()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Shutdown each pool
	for agentName, agentPool := range pm.pools {
		if err := agentPool.pool.Shutdown(ctx); err != nil {
			log.Printf("[pool-manager] Error shutting down pool for %s: %v", agentName, err)
		}
	}

	log.Printf("[pool-manager] All pools shutdown complete")
	return nil
}

// loadScalingConfig loads scaling configuration from agent.yaml or returns defaults.
func (pm *PoolManager) loadScalingConfig(agentPath string) (scaling.ScalingPolicy, int) {
	// Try to load agent.yaml
	agentYAML := filepath.Join(agentPath, "agent.yaml")
	if _, err := os.Stat(agentYAML); err == nil {
		// Parse agent.yaml
		data, readErr := os.ReadFile(agentYAML)
		if readErr == nil {
			var agentCfg struct {
				Scaling struct {
					Tier               string  `yaml:"tier"`
					MinWorkers         int     `yaml:"min_workers"`
					MaxWorkers         int     `yaml:"max_workers"`
					TargetUtilization  float64 `yaml:"target_utilization"`
					ScaleUpThreshold   float64 `yaml:"scale_up_threshold"`
					ScaleDownThreshold float64 `yaml:"scale_down_threshold"`
					ScaleUpDelay       string  `yaml:"scale_up_delay"`
					ScaleDownDelay     string  `yaml:"scale_down_delay"`
					IdleTimeout        string  `yaml:"idle_timeout"`
					QueueSize          int     `yaml:"queue_size"`
				} `yaml:"scaling"`
			}

			if yamlErr := yaml.Unmarshal(data, &agentCfg); yamlErr == nil {
				// If tier specified, use tier defaults as base
				var policy scaling.ScalingPolicy
				queueSize := 10 // Default

				if agentCfg.Scaling.Tier != "" {
					policy = scaling.GetTierPolicy(agentCfg.Scaling.Tier)
					tierCfg := scaling.GetTierConfig(agentCfg.Scaling.Tier)
					queueSize = tierCfg.QueueSize
				} else {
					// No tier, use free tier as baseline
					policy = scaling.GetTierPolicy("free")
					tierCfg := scaling.GetTierConfig("free")
					queueSize = tierCfg.QueueSize
				}

				// Apply explicit overrides
				if agentCfg.Scaling.MinWorkers > 0 {
					policy.MinWorkers = agentCfg.Scaling.MinWorkers
				}
				if agentCfg.Scaling.MaxWorkers > 0 {
					policy.MaxWorkers = agentCfg.Scaling.MaxWorkers
				}
				if agentCfg.Scaling.TargetUtilization > 0 {
					policy.TargetUtilization = agentCfg.Scaling.TargetUtilization
				}
				if agentCfg.Scaling.ScaleUpThreshold > 0 {
					policy.ScaleUpThreshold = agentCfg.Scaling.ScaleUpThreshold
				}
				if agentCfg.Scaling.ScaleDownThreshold > 0 {
					policy.ScaleDownThreshold = agentCfg.Scaling.ScaleDownThreshold
				}
				if agentCfg.Scaling.ScaleUpDelay != "" {
					if d, err := time.ParseDuration(agentCfg.Scaling.ScaleUpDelay); err == nil {
						policy.ScaleUpDelay = d
					}
				}
				if agentCfg.Scaling.ScaleDownDelay != "" {
					if d, err := time.ParseDuration(agentCfg.Scaling.ScaleDownDelay); err == nil {
						policy.ScaleDownDelay = d
					}
				}
				if agentCfg.Scaling.IdleTimeout != "" {
					if d, err := time.ParseDuration(agentCfg.Scaling.IdleTimeout); err == nil {
						policy.IdleTimeout = d
					}
				}
				if agentCfg.Scaling.QueueSize > 0 {
					queueSize = agentCfg.Scaling.QueueSize
				}

				return policy, queueSize
			}
		}
	}

	// Fallback to tier defaults from environment or "free"
	tier := os.Getenv("AGENTSCALE_DEFAULT_TIER")
	if tier == "" {
		tier = "free"
	}

	policy := scaling.GetTierPolicy(tier)
	tierCfg := scaling.GetTierConfig(tier)

	log.Printf("[pool-manager] Using tier '%s' defaults for '%s' (no scaling config in agent.yaml)", tier, filepath.Base(agentPath))

	return policy, tierCfg.QueueSize
}

// DaemonWorkerSpawner creates workers that execute via daemon's Execute() function.
type DaemonWorkerSpawner struct {
	agentName   string
	agentPath   string
	resolvedEnv []string
	counter     atomic.Int64
}

// NewDaemonWorkerSpawner creates a new spawner for daemon-based workers.
func NewDaemonWorkerSpawner(agentName, agentPath string, resolvedEnv []string) *DaemonWorkerSpawner {
	return &DaemonWorkerSpawner{
		agentName:   agentName,
		agentPath:   agentPath,
		resolvedEnv: resolvedEnv,
	}
}

// SpawnWorker creates a new DaemonWorker.
func (s *DaemonWorkerSpawner) SpawnWorker(ctx context.Context, agentID string) (scaling.Worker, error) {
	// Generate unique worker ID
	count := s.counter.Add(1)
	workerID := fmt.Sprintf("%s-worker-%d", agentID, count)

	// Create worker
	worker := &DaemonWorker{
		id:          workerID,
		agentID:     agentID,
		agentPath:   s.agentPath,
		resolvedEnv: s.resolvedEnv,
	}

	// Initialize state
	worker.idle.Store(true)
	worker.health.Store(int32(scaling.HealthHealthy))
	worker.lastUsed.Store(time.Now().UnixNano())

	log.Printf("[worker] Spawned %s for agent '%s'", workerID, agentID)

	return worker, nil
}

// KillWorker terminates a worker.
func (s *DaemonWorkerSpawner) KillWorker(ctx context.Context, workerID string) error {
	// Workers are in-process and managed by the pool.
	// No explicit kill needed - they stop when removed from pool.
	return nil
}

// DaemonWorker implements scaling.Worker by wrapping daemon's Execute() function.
type DaemonWorker struct {
	id          string
	agentID     string
	agentPath   string
	resolvedEnv []string

	lastUsed            atomic.Int64 // Unix nano timestamp
	idle                atomic.Bool  // Currently idle
	health              atomic.Int32 // HealthStatus as int32
	consecutiveFailures atomic.Int32 // Track failures

	mu       sync.Mutex // Protects shutdown state
	shutdown bool
}

// ID returns the worker's unique identifier.
func (w *DaemonWorker) ID() string {
	return w.id
}

// AgentID returns the agent this worker serves.
func (w *DaemonWorker) AgentID() string {
	return w.agentID
}

// IsIdle returns true if the worker is available.
func (w *DaemonWorker) IsIdle() bool {
	return w.idle.Load()
}

// LastUsed returns when the worker was last used.
func (w *DaemonWorker) LastUsed() time.Time {
	return time.Unix(0, w.lastUsed.Load())
}

// Health returns the worker's health status.
func (w *DaemonWorker) Health() scaling.HealthStatus {
	return scaling.HealthStatus(w.health.Load())
}

// Execute runs a task via daemon's Execute() function.
func (w *DaemonWorker) Execute(ctx context.Context, input []byte) (*scaling.Result, error) {
	w.mu.Lock()
	if w.shutdown {
		w.mu.Unlock()
		return nil, fmt.Errorf("worker %s is shut down", w.id)
	}
	w.mu.Unlock()

	// Mark as busy
	w.idle.Store(false)
	defer func() {
		w.idle.Store(true)
		w.lastUsed.Store(time.Now().UnixNano())
	}()

	// Parse input JSON to map
	var inputMap map[string]interface{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &inputMap); err != nil {
			return nil, fmt.Errorf("invalid input JSON: %w", err)
		}
	} else {
		inputMap = make(map[string]interface{})
	}

	// Build environment variables map
	envMap := make(map[string]string)
	for _, envStr := range w.resolvedEnv {
		// Split "KEY=VALUE"
		if idx := len(envStr); idx > 0 {
			// Find first '='
			for i := 0; i < len(envStr); i++ {
				if envStr[i] == '=' {
					key := envStr[:i]
					value := envStr[i+1:]
					envMap[key] = value
					break
				}
			}
		}
	}

	// Build RunRequest for daemon's Execute()
	req := &RunRequest{
		AgentPath: w.agentPath,
		Input:     inputMap,
		Env:       envMap,
	}

	// Execute using daemon's existing infrastructure
	result, err := Execute(ctx, req)
	if err != nil {
		// Track failure
		w.trackFailure()
		return nil, err
	}

	// Check result status
	if result.Status == "error" || result.Status == "timeout" {
		w.trackFailure()
	} else {
		// Success - reset failure counter
		w.consecutiveFailures.Store(0)
		w.health.Store(int32(scaling.HealthHealthy))
	}

	// Convert proxy.Result → scaling.Result
	scalingResult := &scaling.Result{
		Status:   string(result.Status),
		Output:   result.Output,
		Error:    result.Error,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Duration: result.Duration,
	}

	return scalingResult, nil
}

// ExecuteStreaming runs a task via daemon's ExecuteStreaming() with SSE support.
// This method enables pool-based SSE streaming by bridging the channel-based
// worker pool communication with runtime.StreamWriter.
func (w *DaemonWorker) ExecuteStreaming(ctx context.Context, input []byte, streamCh chan *scaling.StreamEvent) (*scaling.Result, error) {
	w.mu.Lock()
	if w.shutdown {
		w.mu.Unlock()
		return nil, fmt.Errorf("worker %s is shut down", w.id)
	}
	w.mu.Unlock()

	// Mark as busy
	w.idle.Store(false)
	defer func() {
		w.idle.Store(true)
		w.lastUsed.Store(time.Now().UnixNano())
	}()

	// Parse input JSON to map
	var inputMap map[string]interface{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &inputMap); err != nil {
			return nil, fmt.Errorf("invalid input JSON: %w", err)
		}
	} else {
		inputMap = make(map[string]interface{})
	}

	// Build environment variables map
	envMap := make(map[string]string)
	for _, envStr := range w.resolvedEnv {
		if idx := len(envStr); idx > 0 {
			for i := 0; i < len(envStr); i++ {
				if envStr[i] == '=' {
					key := envStr[:i]
					value := envStr[i+1:]
					envMap[key] = value
					break
				}
			}
		}
	}

	// Build RunRequest for daemon's ExecuteStreaming()
	req := &RunRequest{
		AgentPath: w.agentPath,
		Input:     inputMap,
		Env:       envMap,
	}

	// Create stream adapter: runtime.StreamWriter → chan *scaling.StreamEvent
	streamWriter := newChannelStreamWriter(streamCh)

	// Execute using daemon's ExecuteStreaming infrastructure
	result, err := ExecuteStreaming(ctx, req, streamWriter)
	if err != nil {
		// Track failure
		w.trackFailure()
		return nil, err
	}

	// Check result status
	if result.Status == "error" || result.Status == "timeout" {
		w.trackFailure()
	} else {
		// Success - reset failure counter
		w.consecutiveFailures.Store(0)
		w.health.Store(int32(scaling.HealthHealthy))
	}

	// Convert proxy.Result → scaling.Result
	scalingResult := &scaling.Result{
		Status:   string(result.Status),
		Output:   result.Output,
		Error:    result.Error,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Duration: result.Duration,
	}

	return scalingResult, nil
}

// trackFailure updates failure counter and health status.
func (w *DaemonWorker) trackFailure() {
	failures := w.consecutiveFailures.Add(1)
	if failures >= 3 {
		// After 3 consecutive failures, mark as unhealthy
		w.health.Store(int32(scaling.HealthUnhealthy))
	} else {
		// First or second failure - mark as degraded
		w.health.Store(int32(scaling.HealthDegraded))
	}
}

// Shutdown gracefully stops the worker.
func (w *DaemonWorker) Shutdown(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.shutdown {
		return nil // Already shut down
	}

	w.shutdown = true
	w.health.Store(int32(scaling.HealthUnhealthy))

	// Daemon workers are stateless - no cleanup needed
	return nil
}
