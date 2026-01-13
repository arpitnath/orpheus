// Package daemon provides the orpheus daemon server.
package daemon

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"orpheus/daemon/pkg/execlog"
	"orpheus/daemon/pkg/mcp"
	"orpheus/daemon/pkg/registry"
	"orpheus/daemon/pkg/scaling"
	"orpheus/daemon/pkg/service"
)

// Server is the orpheus daemon HTTP server.
type Server struct {
	config     *DaemonConfig
	version    string
	listeners  []net.Listener // Multiple listeners (Unix socket + TCP)
	httpServer *http.Server
	startTime  time.Time

	// Agent registry (for discovery and env vars)
	registry registry.Registry

	// ExecLog directory (for execution logging)
	execlogDir string
	retention  *execlog.Retention // ExecLog retention manager

	// Model server management (ServiceManager)
	serviceManager *service.Manager

	// Autoscaling (NEW - integrates pkg/scaling)
	poolManager *PoolManager
	autoscaler  *scaling.BasicAutoscaler

	// Running agents (for status/kill endpoints)
	running map[string]*RunningAgent
	mu      sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// RunningAgent tracks an in-flight agent execution.
type RunningAgent struct {
	ID        string
	AgentPath string
	StartedAt time.Time
	Cancel    context.CancelFunc
}

// NewServer creates a new daemon server with the given configuration.
func NewServer(config *DaemonConfig, version string, execlogDir string) (*Server, error) {
	// Create server context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:     config,
		version:    version,
		execlogDir: execlogDir,
		startTime:  time.Now(),
		running:    make(map[string]*RunningAgent),
		listeners: make([]net.Listener, 0),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Initialize agent registry
	registryPath := "/var/lib/orpheus/registry"
	if _, err := os.Stat("/var/lib/orpheus"); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		registryPath = filepath.Join(home, ".orpheus", "registry")
	}

	reg, err := registry.NewFileRegistry(registryPath)
	if err != nil {
		return nil, fmt.Errorf("init agent registry: %w", err)
	}
	s.registry = reg
	log.Printf("Agent registry initialized at %s", registryPath)

	// Initialize autoscaler (5 second interval for scaling checks)
	autoscaler := scaling.NewAutoscalerWithInterval(5 * time.Second)
	s.autoscaler = autoscaler
	log.Printf("Autoscaler initialized (interval: 5s)")

	// Initialize service manager (model server management)
	serviceManager := service.NewManager()
	s.serviceManager = serviceManager
	log.Printf("ServiceManager initialized (platform-aware model management)")

	// Initialize pool manager (pass serviceManager for model server access)
	poolManager := NewPoolManager(reg, autoscaler, execlogDir, serviceManager, ctx)
	s.poolManager = poolManager
	log.Printf("Pool manager initialized")

	// Initialize execlog retention (30 days, cleanup every 24 hours)
	retention := execlog.NewRetention(execlogDir, 30, 24*time.Hour)
	s.retention = retention
	log.Printf("ExecLog retention initialized (retention=30d, interval=24h)")

	mux := http.NewServeMux()

	// RESTful agent routes
	mux.HandleFunc("/v1/agents/", s.handleAgentResource) // /{name}/run, /{name}/logs, etc.
	mux.HandleFunc("/v1/agents", s.handleAgentsList)     // GET list agents

	// Core endpoints
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/deploy", s.handleDeploy)        // POST /v1/deploy
	mux.HandleFunc("/v1/stats", s.handleStats)          // GET /v1/stats (Phase 3)
	mux.HandleFunc("/v1/execlog/crashed", s.handleExecLogCrashed) // GET crashed requests
	mux.HandleFunc("/v1/execlog/stats", s.handleExecLogStats) // GET execution stats
	mux.HandleFunc("/v1/execlog", s.handleExecLog)      // GET filtered execution logs

	// Initialize MCP endpoints
	mcpGetter := NewDaemonServerGetter(s)
	mcpManager := mcp.NewMCPServerManager(mcpGetter)
	mcpHandler := mcp.NewMCPHandler(mcpManager)
	mux.Handle("/mcp/", mcpHandler)
	log.Printf("MCP endpoints enabled at /mcp/")

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 600 * time.Second, // Long timeout for agent execution
	}

	return s, nil
}

// ListenAndServe starts the server on configured listeners (Unix socket and/or TCP).
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Validate config
	if err := s.config.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Start autoscaler (NEW - for worker pool scaling)
	if s.autoscaler != nil {
		if err := s.autoscaler.Start(s.ctx); err != nil {
			return fmt.Errorf("start autoscaler: %w", err)
		}
		log.Printf("Autoscaler started")
	}

	// Start retention cleanup
	if s.retention != nil {
		if err := s.retention.Start(s.ctx); err != nil {
			return fmt.Errorf("start retention: %w", err)
		}
	}

	// Start service manager (model server management)
	if s.serviceManager != nil {
		if err := s.serviceManager.Start(s.ctx); err != nil {
			return fmt.Errorf("start service manager: %w", err)
		}
		log.Printf("ServiceManager started")
	}

	errChan := make(chan error, 2)

	// Setup Unix socket listener (if enabled)
	if s.config.UnixSocket.Enabled {
		listener, err := s.setupUnixSocketListener()
		if err != nil {
			return fmt.Errorf("unix socket setup: %w", err)
		}
		s.listeners = append(s.listeners, listener)
		log.Printf("Listening on Unix socket: %s", s.config.UnixSocket.Path)

		// Serve Unix socket WITHOUT auth (local trusted)
		go func() {
			errChan <- s.httpServer.Serve(listener)
		}()
	}

	// Setup TCP listener (if enabled)
	if s.config.TCP.Enabled {
		listener, err := s.setupTCPListener()
		if err != nil {
			// Close already created listeners
			s.closeListeners()
			return fmt.Errorf("tcp setup: %w", err)
		}
		s.listeners = append(s.listeners, listener)

		protocol := "http"
		if s.config.TCP.TLS.Enabled {
			protocol = "https"
		}
		log.Printf("Listening on TCP: %s://%s", protocol, s.config.TCP.Bind)

		// Warn if binding to all interfaces without TLS
		if strings.HasPrefix(s.config.TCP.Bind, "0.0.0.0") && !s.config.TCP.TLS.Enabled {
			log.Printf("WARNING: Binding to all network interfaces without TLS")
			log.Printf("WARNING: Server is accessible from network without encryption")
			log.Printf("WARNING: Consider enabling TLS or binding to 127.0.0.1 for local-only access")
		}

		// Serve TCP (no auth for OSS - same handler as Unix socket)
		tcpServer := &http.Server{
			Handler:      s.httpServer.Handler,
			ReadTimeout:  s.httpServer.ReadTimeout,
			WriteTimeout: s.httpServer.WriteTimeout,
		}

		go func() {
			errChan <- tcpServer.Serve(listener)
		}()
	}

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// setupUnixSocketListener creates a Unix socket listener.
func (s *Server) setupUnixSocketListener() (net.Listener, error) {
	socketPath := s.config.UnixSocket.Path

	// Remove existing socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove socket: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	// Set socket permissions (readable/writable by owner only)
	if err := os.Chmod(socketPath, 0600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	return listener, nil
}

// setupTCPListener creates a TCP listener with optional TLS.
func (s *Server) setupTCPListener() (net.Listener, error) {
	bind := s.config.TCP.Bind

	// TLS mode
	if s.config.TCP.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(
			s.config.TCP.TLS.CertFile,
			s.config.TCP.TLS.KeyFile,
		)
		if err != nil {
			return nil, fmt.Errorf("load TLS cert: %w", err)
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12, // Minimum TLS 1.2
		}

		listener, err := tls.Listen("tcp", bind, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("tls listen: %w", err)
		}

		return listener, nil
	}

	// Plain TCP (no TLS)
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, fmt.Errorf("tcp listen: %w", err)
	}

	return listener, nil
}

// closeListeners closes all active listeners.
func (s *Server) closeListeners() {
	for _, listener := range s.listeners {
		listener.Close()
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Printf("Shutting down server...")

	// Stop autoscaler (NEW - stop scaling decisions)
	if s.autoscaler != nil {
		if err := s.autoscaler.Stop(); err != nil {
			log.Printf("Error stopping autoscaler: %v", err)
		}
	}

	// Stop retention cleanup
	if s.retention != nil {
		if err := s.retention.Stop(); err != nil {
			log.Printf("Error stopping retention: %v", err)
		}
	}

	// Stop service manager (model servers)
	if s.serviceManager != nil {
		if err := s.serviceManager.Stop(ctx); err != nil {
			log.Printf("Error stopping service manager: %v", err)
		}
	}

	// Shutdown pool manager (NEW - drain worker pools)
	if s.poolManager != nil {
		if err := s.poolManager.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down pool manager: %v", err)
		}
	}

	// Cancel all running agents (direct executions, not pooled)
	s.mu.Lock()
	for _, agent := range s.running {
		if agent.Cancel != nil {
			agent.Cancel()
		}
	}
	s.mu.Unlock()

	// Cancel server context
	if s.cancel != nil {
		s.cancel()
	}

	// Shutdown HTTP server (closes all listeners)
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	// Remove Unix socket file (if Unix socket was enabled)
	if s.config.UnixSocket.Enabled {
		os.Remove(s.config.UnixSocket.Path)
	}

	log.Printf("Server shutdown complete")
	return nil
}

// registerAgent adds an agent to the running map.
func (s *Server) registerAgent(agent *RunningAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running[agent.ID] = agent
}

// unregisterAgent removes an agent from the running map.
func (s *Server) unregisterAgent(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
}

// getAgent gets a running agent by ID.
func (s *Server) getAgent(id string) (*RunningAgent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.running[id]
	return agent, ok
}

// runningCount returns the number of running agents.
func (s *Server) runningCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.running)
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
