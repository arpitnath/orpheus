// Package daemon provides the agentscale daemon server.
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

	"agentscale/pkg/auth"
)

// Server is the agentscale daemon HTTP server.
type Server struct {
	config     *DaemonConfig
	version    string
	listeners  []net.Listener // Multiple listeners (Unix socket + TCP)
	httpServer *http.Server
	startTime  time.Time

	// Authentication (for TCP endpoints)
	authStore   *auth.Store
	rateLimiter *auth.RateLimiter

	// Running agents (for status/kill endpoints)
	running map[string]*RunningAgent
	mu      sync.RWMutex
}

// RunningAgent tracks an in-flight agent execution.
type RunningAgent struct {
	ID        string
	AgentPath string
	StartedAt time.Time
	Cancel    context.CancelFunc
}

// NewServer creates a new daemon server with the given configuration.
func NewServer(config *DaemonConfig, version string) (*Server, error) {
	s := &Server{
		config:    config,
		version:   version,
		startTime: time.Now(),
		running:   make(map[string]*RunningAgent),
		listeners: make([]net.Listener, 0),
	}

	// Initialize auth if TCP is enabled
	if config.TCP.Enabled {
		// Determine database path
		// Default: /var/lib/agentscale/keys.db or ~/.agentscale/keys.db
		dbPath := "/var/lib/agentscale/keys.db"
		if _, err := os.Stat("/var/lib/agentscale"); os.IsNotExist(err) {
			// Use home directory if /var/lib doesn't exist
			home, _ := os.UserHomeDir()
			dbPath = filepath.Join(home, ".agentscale", "keys.db")

			// Ensure directory exists
			os.MkdirAll(filepath.Dir(dbPath), 0755)
		}

		// Initialize auth store
		store, err := auth.NewStore(dbPath)
		if err != nil {
			return nil, fmt.Errorf("init auth store: %w", err)
		}
		s.authStore = store

		// Initialize rate limiter
		s.rateLimiter = auth.NewRateLimiter()

		log.Printf("Auth enabled: API keys database at %s", dbPath)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agents/run", s.handleRun)
	mux.HandleFunc("/v1/agents/", s.handleAgent) // GET/DELETE /v1/agents/{id}
	mux.HandleFunc("/v1/health", s.handleHealth)

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

		// Serve TCP WITH auth middleware (network untrusted)
		authHandler := auth.AuthMiddleware(s.authStore, s.rateLimiter)(s.httpServer.Handler)
		tcpServer := &http.Server{
			Handler:      authHandler,
			ReadTimeout:  s.httpServer.ReadTimeout,
			WriteTimeout: s.httpServer.WriteTimeout,
		}

		log.Printf("Auth enabled for TCP endpoints (Unix socket remains unauthenticated)")

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
	// Cancel all running agents
	s.mu.Lock()
	for _, agent := range s.running {
		if agent.Cancel != nil {
			agent.Cancel()
		}
	}
	s.mu.Unlock()

	// Shutdown HTTP server (closes all listeners)
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	// Remove Unix socket file (if Unix socket was enabled)
	if s.config.UnixSocket.Enabled {
		os.Remove(s.config.UnixSocket.Path)
	}

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
