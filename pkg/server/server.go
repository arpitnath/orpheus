package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/scaling"
)

// AgentInstance holds all components for one agent deployment.
// Each instance has its own queue, worker pool, and lifecycle management.
type AgentInstance struct {
	agentID string
	cfg     *config.AgentConfig
	queue   *scaling.RequestQueue
	pool    *scaling.BasicWorkerPool

	// Per-agent lifecycle
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWg     sync.WaitGroup
}

// Server orchestrates multiple agent instances with shared autoscaler.
// It handles incoming requests, routes them to the correct agent's queue,
// and manages graceful shutdown of all agents.
type Server struct {
	// Configuration
	serverCfg *config.ServerConfig
	addr      string

	// Multi-agent management
	instances  map[string]*AgentInstance
	autoscaler *scaling.BasicAutoscaler
	mu         sync.RWMutex // Protects instances map

	// HTTP
	httpServer *http.Server
	mux        *http.ServeMux

	// Server-level lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new multi-agent server from the given configuration.
// It initializes queue, worker pool, and autoscaler for each agent.
func New(serverCfg *config.ServerConfig) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Parse autoscaler interval
	autoscalerInterval, err := time.ParseDuration(serverCfg.Server.AutoscalerInterval)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("invalid autoscaler_interval: %w", err)
	}

	// Create shared autoscaler
	autoscaler := scaling.NewAutoscalerWithInterval(autoscalerInterval)
	mux := http.NewServeMux()
	addr := fmt.Sprintf(":%d", serverCfg.Server.Port)

	s := &Server{
		serverCfg:  serverCfg,
		addr:       addr,
		instances:  make(map[string]*AgentInstance),
		autoscaler: autoscaler,
		mux:        mux,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create an AgentInstance for each agent
	for agentID, deployment := range serverCfg.Agents {
		instance, err := s.createAgentInstance(agentID, deployment)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create instance for %s: %w", agentID, err)
		}
		s.instances[agentID] = instance
	}

	// Setup routes
	s.setupRoutes()

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // Allow long-running agent executions
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[server] Created multi-agent server with %d agents on %s", len(s.instances), addr)
	return s, nil
}

// createAgentInstance creates components for a single agent.
func (s *Server) createAgentInstance(
	agentID string,
	deployment config.AgentDeployment,
) (*AgentInstance, error) {
	// Parse scaling config
	parsedScaling, err := config.ParseScalingConfig(deployment.Scaling)
	if err != nil {
		return nil, fmt.Errorf("invalid scaling config: %w", err)
	}

	// Convert to scaling policy
	policy := scaling.ScalingPolicy{
		MinWorkers:         parsedScaling.MinWorkers,
		MaxWorkers:         parsedScaling.MaxWorkers,
		TargetUtilization:  parsedScaling.TargetUtilization,
		ScaleUpThreshold:   parsedScaling.ScaleUpThreshold,
		ScaleDownThreshold: parsedScaling.ScaleDownThreshold,
		ScaleUpDelay:       parsedScaling.ScaleUpDelay,
		ScaleDownDelay:     parsedScaling.ScaleDownDelay,
		IdleTimeout:        parsedScaling.IdleTimeout,
	}

	// Create queue
	queue := scaling.NewRequestQueue(agentID, deployment.Scaling.QueueSize)

	// Create spawner with image path
	spawner := scaling.NewAgentSpawner(deployment.AgentConfig, deployment.ImagePath)

	// Create pool
	pool := scaling.NewWorkerPool(agentID, spawner, policy)

	// Create per-agent context
	workerCtx, workerCancel := context.WithCancel(s.ctx)

	instance := &AgentInstance{
		agentID:      agentID,
		cfg:          deployment.AgentConfig,
		queue:        queue,
		pool:         pool,
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}

	log.Printf("[server] Created instance for '%s' (min=%d, max=%d, queue=%d)",
		agentID, policy.MinWorkers, policy.MaxWorkers, deployment.Scaling.QueueSize)
	return instance, nil
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/invoke", s.handleInvoke)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/stats", s.handleStats)
}

// Start begins the server, starting all components and the HTTP listener.
func (s *Server) Start() error {
	// 1. Start shared autoscaler
	if err := s.autoscaler.Start(s.ctx); err != nil {
		return err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 2. Register each agent with autoscaler and start worker loops
	for agentID, instance := range s.instances {
		deployment := s.serverCfg.Agents[agentID]

		// Parse scaling config to get policy
		parsedScaling, err := config.ParseScalingConfig(deployment.Scaling)
		if err != nil {
			return fmt.Errorf("agent %s: failed to parse scaling config: %w", agentID, err)
		}

		policy := scaling.ScalingPolicy{
			MinWorkers:         parsedScaling.MinWorkers,
			MaxWorkers:         parsedScaling.MaxWorkers,
			TargetUtilization:  parsedScaling.TargetUtilization,
			ScaleUpThreshold:   parsedScaling.ScaleUpThreshold,
			ScaleDownThreshold: parsedScaling.ScaleDownThreshold,
			ScaleUpDelay:       parsedScaling.ScaleUpDelay,
			ScaleDownDelay:     parsedScaling.ScaleDownDelay,
			IdleTimeout:        parsedScaling.IdleTimeout,
		}

		// Register with autoscaler
		s.autoscaler.RegisterPool(agentID, instance.pool, policy)
		s.autoscaler.RegisterQueueMetrics(agentID, instance.queue)

		// Start worker loops for this agent
		numWorkers := instance.pool.Size()
		if numWorkers < 1 {
			numWorkers = 1
		}
		for i := 0; i < numWorkers; i++ {
			instance.workerWg.Add(1)
			go s.workerLoop(instance)
		}

		log.Printf("[server] Started %d workers for '%s'", numWorkers, agentID)
	}

	log.Printf("[server] Starting HTTP server on %s", s.addr)

	// 3. Start HTTP server (blocks until shutdown)
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil // Normal shutdown
	}
	return err
}

// workerLoop processes requests from an agent instance's queue.
// Each loop iteration: dequeue → get worker → execute → return worker → complete.
func (s *Server) workerLoop(instance *AgentInstance) {
	defer instance.workerWg.Done()

	for {
		// Dequeue request from THIS agent's queue
		req, err := instance.queue.Dequeue(instance.workerCtx)
		if err != nil {
			// Context cancelled - shutdown
			return
		}

		// Get worker from THIS agent's pool
		worker, err := instance.pool.GetIdleWorker(req.Context)
		if err != nil {
			// Failed to get worker - send error response
			req.ResponseCh <- &scaling.Response{Error: err}
			instance.queue.Complete(req.ID)
			continue
		}

		// Execute the request
		start := time.Now()
		result, err := worker.Execute(req.Context, req.Input)
		duration := time.Since(start)

		// Log execution result for debugging
		if err != nil {
			log.Printf("[worker] Agent '%s' execution error: %v", instance.agentID, err)
		} else if result.Status != "success" {
			log.Printf("[worker] Agent '%s' returned status=%s, error=%s, stderr=%s",
				instance.agentID, result.Status, result.Error, result.Stderr)
		} else {
			log.Printf("[worker] Agent '%s' completed successfully (duration=%v)",
				instance.agentID, duration)
		}

		// Return worker to pool
		instance.pool.ReturnWorker(worker)

		// Send response
		req.ResponseCh <- &scaling.Response{
			Result:   result,
			Error:    err,
			Duration: duration,
		}

		// Mark request as complete
		instance.queue.Complete(req.ID)
	}
}

// Shutdown gracefully stops the server, draining in-flight requests for all agents.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Printf("[server] Shutting down...")

	// 1. Cancel server context (signals all worker loops to stop)
	s.cancel()

	// 2. Shutdown HTTP server (stops accepting new connections, drains existing)
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("[server] HTTP shutdown error: %v", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 3. Close each agent's queue (stops accepting new requests)
	for agentID, instance := range s.instances {
		instance.queue.Close()
		log.Printf("[server] Closed queue for '%s'", agentID)
	}

	// 4. Wait for all agent workers to drain
	shutdownWg := sync.WaitGroup{}
	for agentID, instance := range s.instances {
		shutdownWg.Add(1)
		go func(id string, inst *AgentInstance) {
			defer shutdownWg.Done()
			done := make(chan struct{})
			go func() {
				inst.workerWg.Wait()
				close(done)
			}()
			select {
			case <-done:
				log.Printf("[server] Agent '%s' workers drained", id)
			case <-ctx.Done():
				log.Printf("[server] Agent '%s' timeout", id)
			}
		}(agentID, instance)
	}
	shutdownWg.Wait()

	// 5. Stop autoscaler
	if err := s.autoscaler.Stop(); err != nil {
		log.Printf("[server] Autoscaler stop error: %v", err)
	}

	// 6. Shutdown each pool
	for agentID, instance := range s.instances {
		if err := instance.pool.Shutdown(ctx); err != nil {
			log.Printf("[server] Pool shutdown error for '%s': %v", agentID, err)
		}
	}

	log.Printf("[server] Shutdown complete")
	return nil
}

// Addr returns the server's listen address.
func (s *Server) Addr() string {
	return s.addr
}

// GetAgentInstance returns an agent instance by ID (for handlers).
func (s *Server) getAgentInstance(agentID string) (*AgentInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instance, ok := s.instances[agentID]
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", agentID)
	}
	return instance, nil
}

// ListAgents returns a list of all agent IDs.
func (s *Server) listAgents() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]string, 0, len(s.instances))
	for agentID := range s.instances {
		agents = append(agents, agentID)
	}
	return agents
}
