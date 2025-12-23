package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RunRequest is the request body for POST /v1/agents/run.
type RunRequest struct {
	AgentPath string                 `json:"agent_path"`
	Input     map[string]interface{} `json:"input"`
	Options   RunOptions             `json:"options"`
}

// RunOptions are optional execution parameters.
type RunOptions struct {
	MemoryLimit int `json:"memory_limit"` // MB
	Timeout     int `json:"timeout"`      // seconds
	IdleTimeout int `json:"idle_timeout"` // seconds
}

// RunResponse is the response body for POST /v1/agents/run.
type RunResponse struct {
	Status     string                 `json:"status"`
	Output     map[string]interface{} `json:"output,omitempty"`
	RawOutput  string                 `json:"raw_output,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Stderr     string                 `json:"stderr,omitempty"`
	DurationMs int64                  `json:"duration_ms"`
}

// AgentStatusResponse is the response for GET /v1/agents/{id}.
type AgentStatusResponse struct {
	ID        string `json:"id"`
	AgentPath string `json:"agent_path"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at"`
	RunningMs int64  `json:"running_ms"`
}

// HealthResponse is the response for GET /v1/health.
type HealthResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	RunningAgents int    `json:"running_agents"`
}

// handleRun handles POST /v1/agents/run.
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse request
	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	if req.AgentPath == "" {
		writeError(w, http.StatusBadRequest, "agent_path is required")
		return
	}

	// Generate agent ID
	agentID := fmt.Sprintf("agent-%s", uuid.New().String()[:8])

	// Create cancellable context
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Apply timeout if specified
	if req.Options.Timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, time.Duration(req.Options.Timeout)*time.Second)
		defer timeoutCancel()
	}

	// Register running agent
	runningAgent := &RunningAgent{
		ID:        agentID,
		AgentPath: req.AgentPath,
		StartedAt: time.Now(),
		Cancel:    cancel,
	}
	s.registerAgent(runningAgent)
	defer s.unregisterAgent(agentID)

	// Execute agent
	result, err := Execute(ctx, &req)
	if err != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Status:     "error",
			Error:      err.Error(),
			DurationMs: time.Since(runningAgent.StartedAt).Milliseconds(),
		})
		return
	}

	// Convert proxy.Result to RunResponse
	resp := RunResponse{
		Status:     string(result.Status),
		Output:     result.Output,
		RawOutput:  result.RawOutput,
		Error:      result.Error,
		Stderr:     result.Stderr,
		DurationMs: result.Duration.Milliseconds(),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAgent handles GET/DELETE /v1/agents/{id}.
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path: /v1/agents/{id}
	path := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	agentID := strings.TrimSuffix(path, "/")

	if agentID == "" {
		// List all running agents
		if r.Method == http.MethodGet {
			s.handleListAgents(w, r)
			return
		}
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetAgent(w, agentID)
	case http.MethodDelete:
		s.handleKillAgent(w, agentID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGetAgent handles GET /v1/agents/{id}.
func (s *Server) handleGetAgent(w http.ResponseWriter, agentID string) {
	agent, ok := s.getAgent(agentID)
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	resp := AgentStatusResponse{
		ID:        agent.ID,
		AgentPath: agent.AgentPath,
		Status:    "running",
		StartedAt: agent.StartedAt.Format(time.RFC3339),
		RunningMs: time.Since(agent.StartedAt).Milliseconds(),
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleKillAgent handles DELETE /v1/agents/{id}.
func (s *Server) handleKillAgent(w http.ResponseWriter, agentID string) {
	agent, ok := s.getAgent(agentID)
	if !ok {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Cancel the agent's context
	if agent.Cancel != nil {
		agent.Cancel()
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "cancelled",
		"agent_id": agentID,
	})
}

// handleListAgents handles GET /v1/agents/.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]AgentStatusResponse, 0, len(s.running))
	for _, agent := range s.running {
		agents = append(agents, AgentStatusResponse{
			ID:        agent.ID,
			AgentPath: agent.AgentPath,
			Status:    "running",
			StartedAt: agent.StartedAt.Format(time.RFC3339),
			RunningMs: time.Since(agent.StartedAt).Milliseconds(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	})
}

// handleHealth handles GET /v1/health.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := HealthResponse{
		Status:        "healthy",
		Version:       s.version,
		UptimeSeconds: int64(time.Since(s.startTime).Seconds()),
		RunningAgents: s.runningCount(),
	}
	writeJSON(w, http.StatusOK, resp)
}
