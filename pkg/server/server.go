package server

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/scaling"
)

// Server orchestrates Queue, WorkerPool, and Autoscaler with HTTP endpoints.
// It handles incoming requests, routes them through the queue to workers,
// and manages graceful shutdown.
type Server struct {
	// Configuration
	cfg  *config.AgentConfig
	addr string
	tier string

	// Components
	queue      *scaling.RequestQueue
	pool       *scaling.BasicWorkerPool
	autoscaler *scaling.BasicAutoscaler

	// HTTP
	httpServer *http.Server
	mux        *http.ServeMux

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new server for the given agent configuration.
// It initializes the queue, worker pool, and autoscaler components.
func New(cfg *config.AgentConfig, addr string, tier string) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Get tier configuration
	tierConfig := scaling.GetTierConfig(tier)
	policy := tierConfig.ToScalingPolicy()

	// Create components
	queue := scaling.NewRequestQueue(cfg.Name, tierConfig.QueueSize)
	spawner := scaling.NewAgentSpawner(cfg)
	pool := scaling.NewWorkerPool(cfg.Name, spawner, policy)
	autoscaler := scaling.NewAutoscaler()

	// Create HTTP mux
	mux := http.NewServeMux()

	s := &Server{
		cfg:        cfg,
		addr:       addr,
		tier:       tier,
		queue:      queue,
		pool:       pool,
		autoscaler: autoscaler,
		mux:        mux,
		ctx:        ctx,
		cancel:     cancel,
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

	log.Printf("[server] Created server for agent %s (tier=%s, addr=%s)", cfg.Name, tier, addr)
	return s, nil
}

// setupRoutes configures the HTTP routes.
func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/invoke", s.handleInvoke)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/stats", s.handleStats)
}

// Start begins the server, starting all components and the HTTP listener.
func (s *Server) Start() error {
	// 1. Start autoscaler
	if err := s.autoscaler.Start(s.ctx); err != nil {
		return err
	}

	// 2. Register pool and queue metrics with autoscaler
	policy := scaling.GetTierPolicy(s.tier)
	s.autoscaler.RegisterPool(s.cfg.Name, s.pool, policy)
	s.autoscaler.RegisterQueueMetrics(s.cfg.Name, s.queue)

	// 3. Start worker loop goroutines
	// Start with number equal to initial pool size
	numWorkers := s.pool.Size()
	if numWorkers < 1 {
		numWorkers = 1
	}
	for i := 0; i < numWorkers; i++ {
		s.wg.Add(1)
		go s.workerLoop()
	}

	log.Printf("[server] Starting HTTP server on %s (workers=%d)", s.addr, numWorkers)

	// 4. Start HTTP server (blocks until shutdown)
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil // Normal shutdown
	}
	return err
}

// workerLoop processes requests from the queue.
// Each loop iteration: dequeue → get worker → execute → return worker → complete.
func (s *Server) workerLoop() {
	defer s.wg.Done()

	for {
		// Dequeue request (blocks until available or context cancelled)
		req, err := s.queue.Dequeue(s.ctx)
		if err != nil {
			// Context cancelled - shutdown
			return
		}

		// Get worker from pool
		worker, err := s.pool.GetIdleWorker(req.Context)
		if err != nil {
			// Failed to get worker - send error response
			req.ResponseCh <- &scaling.Response{Error: err}
			s.queue.Complete(req.ID)
			continue
		}

		// Execute the request
		start := time.Now()
		result, err := worker.Execute(req.Context, req.Input)
		duration := time.Since(start)

		// Return worker to pool
		s.pool.ReturnWorker(worker)

		// Send response
		req.ResponseCh <- &scaling.Response{
			Result:   result,
			Error:    err,
			Duration: duration,
		}

		// Mark request as complete
		s.queue.Complete(req.ID)
	}
}

// Shutdown gracefully stops the server, draining in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Printf("[server] Shutting down...")

	// 1. Cancel server context (signals worker loops to stop)
	s.cancel()

	// 2. Shutdown HTTP server (stops accepting new connections, drains existing)
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("[server] HTTP shutdown error: %v", err)
	}

	// 3. Close queue (stops accepting new requests)
	s.queue.Close()

	// 4. Wait for worker loops to drain
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[server] Worker loops drained")
	case <-ctx.Done():
		log.Printf("[server] Timeout waiting for workers to drain")
	}

	// 5. Stop autoscaler
	if err := s.autoscaler.Stop(); err != nil {
		log.Printf("[server] Autoscaler stop error: %v", err)
	}

	// 6. Shutdown worker pool
	if err := s.pool.Shutdown(ctx); err != nil {
		log.Printf("[server] Pool shutdown error: %v", err)
	}

	log.Printf("[server] Shutdown complete")
	return nil
}

// Addr returns the server's listen address.
func (s *Server) Addr() string {
	return s.addr
}

// AgentID returns the agent ID this server is serving.
func (s *Server) AgentID() string {
	return s.cfg.Name
}
