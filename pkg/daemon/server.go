// Package daemon provides the agentscale daemon server.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Server is the agentscale daemon HTTP server.
type Server struct {
	socketPath string
	version    string
	listener   net.Listener
	httpServer *http.Server
	startTime  time.Time

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

// NewServer creates a new daemon server.
func NewServer(socketPath, version string) *Server {
	s := &Server{
		socketPath: socketPath,
		version:    version,
		startTime:  time.Now(),
		running:    make(map[string]*RunningAgent),
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

	return s
}

// ListenAndServe starts the server on the Unix socket.
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Remove existing socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove socket: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = listener

	// Set socket permissions (readable/writable by owner)
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	// Start serving
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.httpServer.Serve(listener)
	}()

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

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	// Remove socket file
	os.Remove(s.socketPath)
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
