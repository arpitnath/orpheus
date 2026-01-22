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

	"orpheus/daemon/pkg/config"
	"orpheus/daemon/pkg/execlog"
	"orpheus/daemon/pkg/registry"
	"orpheus/daemon/pkg/scaling"
	"orpheus/daemon/pkg/service"

	"gopkg.in/yaml.v3"
)

// AgentPool holds all components for one agent's autoscaling.
type AgentPool struct {
	agentName string
	agentPath string
	queue     *scaling.RequestQueue
	pool      *scaling.BasicWorkerPool
	policy    scaling.ScalingPolicy

	// Session affinity configuration
	sessionConfig config.SessionConfig

	// Per-agent lifecycle
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWg     sync.WaitGroup
}

// PoolManager manages autoscaling pools for all deployed agents.
// It creates pools on-demand when agents are deployed via the registry.
type PoolManager struct {
	registry       registry.Registry
	autoscaler     *scaling.BasicAutoscaler
	execlogDir     string // ExecLog directory for logging
	serviceManager *service.Manager // Model server management

	pools map[string]*AgentPool // agentName → pool
	mu    sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewPoolManager creates a new pool manager.
func NewPoolManager(registry registry.Registry, autoscaler *scaling.BasicAutoscaler, execlogDir string, serviceManager *service.Manager, ctx context.Context) *PoolManager {
	ctx, cancel := context.WithCancel(ctx)
	return &PoolManager{
		execlogDir:     execlogDir,
		registry:       registry,
		autoscaler:     autoscaler,
		serviceManager: serviceManager,
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

	// Create spawner (pass serviceManager for model server access)
	spawner := NewDaemonWorkerSpawner(agentName, agent.Path, agent.ResolvedEnv, pm.serviceManager)

	// Create pool
	pool := scaling.NewWorkerPool(agentName, spawner, policy)

	// Create per-agent context
	workerCtx, workerCancel := context.WithCancel(pm.ctx)

	// Load session affinity config from agent.yaml
	sessionConfig := pm.loadSessionConfig(agent.Path)

	agentPool := &AgentPool{
		agentName:     agentName,
		agentPath:     agent.Path,
		queue:         queue,
		pool:          pool,
		policy:        policy,
		sessionConfig: sessionConfig,
		workerCtx:     workerCtx,
		workerCancel:  workerCancel,
	}

	// Register with autoscaler
	pm.autoscaler.RegisterPool(agentName, pool, policy)
	pm.autoscaler.RegisterQueueMetrics(agentName, queue)

	// Start worker loops (need MaxWorkers goroutines for full concurrency)
	// Each goroutine can independently dequeue and dispatch work
	numLoops := policy.MaxWorkers
	if numLoops < 1 {
		numLoops = 1
	}

	for i := 0; i < numLoops; i++ {
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

// GetAllPools returns a copy of all agent pools (for telemetry/monitoring).
// Returns a map of agentName → AgentPool.
func (pm *PoolManager) GetAllPools() map[string]*AgentPool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pools := make(map[string]*AgentPool, len(pm.pools))
	for name, pool := range pm.pools {
		pools[name] = pool
	}
	return pools
}

// Registry returns the agent registry (for telemetry to list agents).
func (pm *PoolManager) Registry() registry.Registry {
	return pm.registry
}

// GetPoolStats returns worker pool statistics (for telemetry).
func (ap *AgentPool) GetPoolStats() scaling.PoolStats {
	return ap.pool.GetStats()
}

// GetQueueStats returns request queue statistics (for telemetry).
func (ap *AgentPool) GetQueueStats() scaling.QueueStats {
	return ap.queue.GetStats()
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
		// Use session-aware acquisition if session affinity is enabled
		var worker scaling.Worker
		if req.SessionID != "" && agentPool.sessionConfig.Enabled {
			worker, err = agentPool.pool.AcquireForSession(
				req.Context,
				req.SessionID,
				agentPool.sessionConfig.WaitTimeout,
			)
		} else {
			worker, err = agentPool.pool.GetIdleWorker(req.Context)
		}
		if err != nil {
			// Failed to get worker - send error response
			req.ResponseCh <- &scaling.Response{
				Error:    err,
				Duration: 0,
			}
			agentPool.queue.Complete(req.ID)
			continue
		}

		// Log STARTED state (best-effort, async)
		go pm.logExecLogEvent(agentPool.agentName, &execlog.Event{
			RequestID: req.ID,
			State:     execlog.StateStarted,
			WorkerID:  ptrString(worker.ID()),
			SessionID: ptrOrNil(req.SessionID),
		})

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

	// Log COMPLETED or FAILED state (best-effort, async)
	// Check both err != nil AND result.Status != "success" to catch OOM/timeout
	if err != nil || (result != nil && result.Status != "success") {
		var errPtr *string
		if err != nil {
			errPtr = ptrString(err.Error())
		} else if result != nil && result.Error != "" {
			errPtr = ptrString(result.Error)
		}
		go pm.logExecLogEvent(agentPool.agentName, &execlog.Event{
			RequestID:  req.ID,
			State:      execlog.StateFailed,
			WorkerID:   ptrString(worker.ID()),
			DurationMs: ptrInt64(duration.Milliseconds()),
			Error:      errPtr,
		})
	} else {
		go pm.logExecLogEvent(agentPool.agentName, &execlog.Event{
			RequestID:  req.ID,
			State:      execlog.StateCompleted,
			WorkerID:   ptrString(worker.ID()),
			DurationMs: ptrInt64(duration.Milliseconds()),
		})
	}

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

	// Log COMPLETED or FAILED state (best-effort, async)
	// Check both err != nil AND result.Status != "success" to catch OOM/timeout
	if err != nil || (result != nil && result.Status != "success") {
		var errPtr *string
		if err != nil {
			errPtr = ptrString(err.Error())
		} else if result != nil && result.Error != "" {
			errPtr = ptrString(result.Error)
		}
		go pm.logExecLogEvent(agentPool.agentName, &execlog.Event{
			RequestID:  req.ID,
			State:      execlog.StateFailed,
			WorkerID:   ptrString(worker.ID()),
			DurationMs: ptrInt64(duration.Milliseconds()),
			Error:      errPtr,
		})
	} else {
		go pm.logExecLogEvent(agentPool.agentName, &execlog.Event{
			RequestID:  req.ID,
			State:      execlog.StateCompleted,
			WorkerID:   ptrString(worker.ID()),
			DurationMs: ptrInt64(duration.Milliseconds()),
		})
	}

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

// OSS safety limits (prevent resource exhaustion on self-hosted)
const (
	MaxMinWorkers = 50   // Maximum min_workers value
	MaxMaxWorkers = 100  // Maximum max_workers value
	MaxQueueSize  = 1000 // Maximum queue_size value
	MinQueueSize  = 1    // Minimum queue_size value
)

// loadScalingConfig loads scaling configuration from agent.yaml or returns defaults.
// OSS uses one sensible default. Cloud can override with org-specific tiers.
func (pm *PoolManager) loadScalingConfig(agentPath string) (scaling.ScalingPolicy, int) {
	// Try to load explicit scaling config from agent.yaml
	agentYAML := filepath.Join(agentPath, "agent.yaml")
	if _, err := os.Stat(agentYAML); err == nil {
		data, readErr := os.ReadFile(agentYAML)
		if readErr == nil {
			var agentCfg struct {
				Scaling struct {
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
				// Check if any explicit config provided
				hasExplicitConfig := agentCfg.Scaling.MinWorkers > 0 ||
					agentCfg.Scaling.MaxWorkers > 0 ||
					agentCfg.Scaling.QueueSize > 0

				if hasExplicitConfig {
					// Build policy from explicit values + defaults for missing fields
					policy, queueSize := pm.buildPolicyFromValues(
						agentCfg.Scaling.MinWorkers,
						agentCfg.Scaling.MaxWorkers,
						agentCfg.Scaling.TargetUtilization,
						agentCfg.Scaling.ScaleUpThreshold,
						agentCfg.Scaling.ScaleDownThreshold,
						agentCfg.Scaling.ScaleUpDelay,
						agentCfg.Scaling.ScaleDownDelay,
						agentCfg.Scaling.IdleTimeout,
						agentCfg.Scaling.QueueSize,
					)

					// Validate configuration
					if err := validateScalingConfig(policy, queueSize); err != nil {
						log.Printf("[pool-manager] Invalid scaling config in %s: %v", agentYAML, err)
						log.Printf("[pool-manager] Using default policy instead")
						return getDefaultPolicy()
					}

					log.Printf("[pool-manager] Using explicit scaling config for '%s' (min=%d, max=%d, queue=%d)",
						filepath.Base(agentPath), policy.MinWorkers, policy.MaxWorkers, queueSize)
					return policy, queueSize
				}
			}
		}
	}

	// No explicit config - use sensible defaults (OSS: one policy for everyone)
	policy, queueSize := getDefaultPolicy()
	log.Printf("[pool-manager] Using default policy for '%s' (min=%d, max=%d, queue=%d)",
		filepath.Base(agentPath), policy.MinWorkers, policy.MaxWorkers, queueSize)
	return policy, queueSize
}

// getDefaultPolicy returns the sensible default scaling policy for OSS.
// This works for 90% of use cases. Power users can customize via agent.yaml.
func getDefaultPolicy() (scaling.ScalingPolicy, int) {
	return scaling.ScalingPolicy{
		MinWorkers:         1,
		MaxWorkers:         10,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
		ScaleUpDelay:       15 * time.Second,
		ScaleDownDelay:     1 * time.Minute,
		IdleTimeout:        10 * time.Minute,
	}, 50 // queue_size
}

// buildPolicyFromValues builds a scaling policy from agent.yaml explicit values.
// Missing fields use defaults from getDefaultPolicy().
func (pm *PoolManager) buildPolicyFromValues(
	minWorkers int,
	maxWorkers int,
	targetUtilization float64,
	scaleUpThreshold float64,
	scaleDownThreshold float64,
	scaleUpDelay string,
	scaleDownDelay string,
	idleTimeout string,
	queueSize int,
) (scaling.ScalingPolicy, int) {
	// Start with defaults
	defaultPolicy, defaultQueueSize := getDefaultPolicy()
	policy := defaultPolicy

	// Override with explicit values
	if minWorkers > 0 {
		policy.MinWorkers = minWorkers
	}
	if maxWorkers > 0 {
		policy.MaxWorkers = maxWorkers
	}
	if targetUtilization > 0 {
		policy.TargetUtilization = targetUtilization
	}
	if scaleUpThreshold > 0 {
		policy.ScaleUpThreshold = scaleUpThreshold
	}
	if scaleDownThreshold > 0 {
		policy.ScaleDownThreshold = scaleDownThreshold
	}
	if scaleUpDelay != "" {
		if d, err := time.ParseDuration(scaleUpDelay); err == nil {
			policy.ScaleUpDelay = d
		}
	}
	if scaleDownDelay != "" {
		if d, err := time.ParseDuration(scaleDownDelay); err == nil {
			policy.ScaleDownDelay = d
		}
	}
	if idleTimeout != "" {
		if d, err := time.ParseDuration(idleTimeout); err == nil {
			policy.IdleTimeout = d
		}
	}

	resultQueueSize := defaultQueueSize
	if queueSize > 0 {
		resultQueueSize = queueSize
	}

	return policy, resultQueueSize
}

// loadSessionConfig loads session affinity configuration from agent.yaml or returns defaults.
func (pm *PoolManager) loadSessionConfig(agentPath string) config.SessionConfig {
	sessionConfig := config.SessionConfig{}

	// Try to load explicit session config from agent.yaml
	agentYAML := filepath.Join(agentPath, "agent.yaml")
	if _, err := os.Stat(agentYAML); err == nil {
		data, readErr := os.ReadFile(agentYAML)
		if readErr == nil {
			var agentCfg struct {
				Session struct {
					Enabled     bool   `yaml:"enabled"`
					Key         string `yaml:"key"`
					TTL         string `yaml:"ttl"`
					WaitTimeout string `yaml:"wait_timeout"`
				} `yaml:"session"`
			}

			if yamlErr := yaml.Unmarshal(data, &agentCfg); yamlErr == nil {
				sessionConfig.Enabled = agentCfg.Session.Enabled
				sessionConfig.Key = agentCfg.Session.Key

				// Parse durations
				if agentCfg.Session.TTL != "" {
					if d, err := time.ParseDuration(agentCfg.Session.TTL); err == nil {
						sessionConfig.TTL = d
					}
				}
				if agentCfg.Session.WaitTimeout != "" {
					if d, err := time.ParseDuration(agentCfg.Session.WaitTimeout); err == nil {
						sessionConfig.WaitTimeout = d
					}
				}
			}
		}
	}

	// Apply defaults for missing fields
	sessionConfig.SetDefaults()

	if sessionConfig.Enabled {
		log.Printf("[pool-manager] Session affinity enabled for '%s' (key=%s, ttl=%v, wait_timeout=%v)",
			filepath.Base(agentPath), sessionConfig.Key, sessionConfig.TTL, sessionConfig.WaitTimeout)
	}

	return sessionConfig
}

// validateScalingConfig validates a scaling configuration against OSS safety limits.
func validateScalingConfig(policy scaling.ScalingPolicy, queueSize int) error {
	// Worker bounds
	if policy.MinWorkers < 0 {
		return fmt.Errorf("min_workers must be >= 0 (got %d)", policy.MinWorkers)
	}
	if policy.MaxWorkers < 1 {
		return fmt.Errorf("max_workers must be >= 1 (got %d)", policy.MaxWorkers)
	}
	if policy.MinWorkers > policy.MaxWorkers {
		return fmt.Errorf("min_workers (%d) cannot be > max_workers (%d)",
			policy.MinWorkers, policy.MaxWorkers)
	}

	// OSS resource limits (prevent abuse on self-hosted)
	if policy.MinWorkers > MaxMinWorkers {
		return fmt.Errorf("min_workers exceeds OSS limit (%d > %d)", policy.MinWorkers, MaxMinWorkers)
	}
	if policy.MaxWorkers > MaxMaxWorkers {
		return fmt.Errorf("max_workers exceeds OSS limit (%d > %d)", policy.MaxWorkers, MaxMaxWorkers)
	}

	// Queue bounds
	if queueSize < MinQueueSize || queueSize > MaxQueueSize {
		return fmt.Errorf("queue_size must be between %d and %d (got %d)",
			MinQueueSize, MaxQueueSize, queueSize)
	}

	// Threshold logic validation
	if policy.ScaleUpThreshold <= policy.ScaleDownThreshold {
		return fmt.Errorf("scale_up_threshold (%.1f) must be > scale_down_threshold (%.1f)",
			policy.ScaleUpThreshold, policy.ScaleDownThreshold)
	}

	// Threshold sanity (prevent constant scaling)
	if policy.ScaleUpThreshold < 0.1 || policy.ScaleUpThreshold > 50.0 {
		return fmt.Errorf("scale_up_threshold must be between 0.1 and 50.0 (got %.1f)", policy.ScaleUpThreshold)
	}
	if policy.ScaleDownThreshold < 0.0 || policy.ScaleDownThreshold > 10.0 {
		return fmt.Errorf("scale_down_threshold must be between 0.0 and 10.0 (got %.1f)", policy.ScaleDownThreshold)
	}

	// Delay bounds
	if policy.ScaleUpDelay < time.Second {
		return fmt.Errorf("scale_up_delay must be >= 1s (got %v)", policy.ScaleUpDelay)
	}
	if policy.ScaleDownDelay < time.Second {
		return fmt.Errorf("scale_down_delay must be >= 1s (got %v)", policy.ScaleDownDelay)
	}
	if policy.ScaleUpDelay > 10*time.Minute {
		return fmt.Errorf("scale_up_delay must be <= 10m (got %v)", policy.ScaleUpDelay)
	}
	if policy.ScaleDownDelay > 30*time.Minute {
		return fmt.Errorf("scale_down_delay must be <= 30m (got %v)", policy.ScaleDownDelay)
	}

	return nil
}

// DaemonWorkerSpawner creates workers that execute via daemon's Execute() function.
type DaemonWorkerSpawner struct {
	agentName      string
	agentPath      string
	resolvedEnv    []string
	serviceManager *service.Manager // Model server management
	counter        atomic.Int64
}

// NewDaemonWorkerSpawner creates a new spawner for daemon-based workers.
func NewDaemonWorkerSpawner(agentName, agentPath string, resolvedEnv []string, serviceManager *service.Manager) *DaemonWorkerSpawner {
	return &DaemonWorkerSpawner{
		agentName:      agentName,
		agentPath:      agentPath,
		resolvedEnv:    resolvedEnv,
		serviceManager: serviceManager,
	}
}

// SpawnWorker creates a new DaemonWorker.
func (s *DaemonWorkerSpawner) SpawnWorker(ctx context.Context, agentID string) (scaling.Worker, error) {
	// Generate unique worker ID
	count := s.counter.Add(1)
	workerID := fmt.Sprintf("%s-worker-%d", agentID, count)

	// Create worker (pass serviceManager for model server access)
	worker := &DaemonWorker{
		id:             workerID,
		agentID:        agentID,
		agentPath:      s.agentPath,
		resolvedEnv:    s.resolvedEnv,
		serviceManager: s.serviceManager,
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
	id             string
	agentID        string
	agentPath      string
	resolvedEnv    []string
	serviceManager *service.Manager // Model server management

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

	// Execute using daemon's existing infrastructure (pass serviceManager for model server)
	result, err := Execute(ctx, req, w.serviceManager)
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

	// Execute using daemon's ExecuteStreaming infrastructure (pass serviceManager for model server)
	result, err := ExecuteStreaming(ctx, req, streamWriter, w.serviceManager)
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

// logExecLogEvent logs an execution event (best-effort, never fails)
func (pm *PoolManager) logExecLogEvent(agentName string, event *execlog.Event) {
	writer, err := execlog.NewWriter(pm.execlogDir, agentName)
	if err != nil {
		log.Printf("Warning: ExecLog writer error: %v", err)
		return
	}
	// Don't close - writer is cached and reused

	event.Timestamp = time.Now()
	if err := writer.Log(event); err != nil {
		log.Printf("Warning: ExecLog write error: %v", err)
	}
}

// Helper functions for pointer creation
func ptrString(s string) *string {
	return &s
}

func ptrInt64(i int64) *int64 {
	return &i
}
