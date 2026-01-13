package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"orpheus/daemon/pkg/execlog"
	"orpheus/daemon/pkg/proxy"
	"orpheus/daemon/pkg/registry"
	"orpheus/daemon/pkg/runtime"
	"orpheus/daemon/pkg/scaling"

	"github.com/google/uuid"
)

// handleAgentResource routes RESTful agent requests based on path.
// Routes: /v1/agents/{name}/run, /v1/agents/{name}/logs, etc.
func (s *Server) handleAgentResource(w http.ResponseWriter, r *http.Request) {
	// Extract agent name from path: /v1/agents/{name}/action
	path := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	parts := strings.Split(path, "/")

	if len(parts) < 1 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "agent name required")
		return
	}

	agentName := parts[0]

	// Route based on action
	if len(parts) >= 2 {
		action := parts[1]

		switch action {
		case "run":
			if r.Method == http.MethodPost {
				s.handleRunByName(w, r, agentName)
				return
			}
		case "logs":
			if r.Method == http.MethodGet {
				s.handleAgentLogs(w, r, agentName)
				return
			}
		case "stats":
			if r.Method == http.MethodGet {
				s.handleAgentStats(w, r, agentName)
				return
			}
		case "workspace":
			if r.Method == http.MethodGet {
				s.handleWorkspaceInfo(w, r, agentName)
				return
			}
			if r.Method == http.MethodDelete {
				s.handleWorkspaceClean(w, r, agentName)
				return
			}
		}
	}

	// Default: Get agent details
	if r.Method == http.MethodGet {
		s.handleGetAgentByName(w, agentName)
		return
	}

	if r.Method == http.MethodDelete {
		s.handleDeleteAgentByName(w, agentName)
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// handleRunByName executes an agent by name (RESTful endpoint).
// POST /v1/agents/{name}/run
func (s *Server) handleRunByName(w http.ResponseWriter, r *http.Request, agentName string) {
	// Get agent from registry
	agent, err := s.registry.Get(agentName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	// Check if streaming requested (Phase 3)
	isStreaming := r.Header.Get("Accept") == "text/event-stream"

	// Try pool-based execution first
	if s.poolManager != nil {
		pool, poolErr := s.poolManager.GetPool(agentName)
		if poolErr == nil {
			// Pool exists - use pool-based execution
			if isStreaming {
				s.executeViaPoolStreaming(w, r, agentName, pool)
			} else {
				s.executeViaPool(w, r, agentName, pool)
			}
			return
		}

		// Pool doesn't exist - try lazy creation
		if createErr := s.poolManager.CreatePool(agentName); createErr == nil {
			if pool, _ := s.poolManager.GetPool(agentName); pool != nil {
				// Pool created successfully
				if isStreaming {
					s.executeViaPoolStreaming(w, r, agentName, pool)
				} else {
					s.executeViaPool(w, r, agentName, pool)
				}
				return
			}
		}

		// Pool creation failed - log and fall back to direct execution
		log.Printf("[handler] Pool unavailable for '%s', using direct execution", agentName)
	}

	// FALLBACK: Direct execution
	if isStreaming {
		s.handleRunByNameStreaming(w, r, agent)
	} else {
		s.executeDirectly(w, r, agent)
	}
}

// executeViaPool executes an agent request via worker pool (autoscaled).
func (s *Server) executeViaPool(w http.ResponseWriter, r *http.Request, agentName string, pool *AgentPool) {
	// Parse request
	var req struct {
		Input json.RawMessage   `json:"input"`
		Env   map[string]string `json:"env,omitempty"` // Runtime overrides
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Extract session ID from header for session affinity (Phase 2)
	sessionID := r.Header.Get(pool.sessionConfig.Key)

	// Create scaling request
	scalingReq := &scaling.Request{
		ID:         uuid.New().String(),
		Input:      req.Input,
		Context:    r.Context(),
		ResponseCh: make(chan *scaling.Response, 1),
		SessionID:  sessionID,
	}

	// Log QUEUED state (best-effort, async)
	go func() {
		writer, err := execlog.NewWriter(s.execlogDir, agentName)
		if err != nil {
			log.Printf("Warning: Failed to create execlog writer: %v", err)
			return
		}
		// Don't close - writer is cached and reused

		err = writer.Log(&execlog.Event{
			Timestamp: time.Now(),
			RequestID: scalingReq.ID,
			State:     execlog.StateQueued,
			SessionID: ptrOrNil(scalingReq.SessionID),
		})
		if err != nil {
			log.Printf("Warning: Failed to log QUEUED: %v", err)
		}
	}()

	// Enqueue to agent's queue
	if err := pool.queue.Enqueue(r.Context(), scalingReq); err != nil {
		writeError(w, http.StatusServiceUnavailable, "agent queue full")
		return
	}

	// Wait for worker to process
	select {
	case resp := <-scalingReq.ResponseCh:
		if resp.Error != nil {
			writeJSON(w, http.StatusOK, RunResponse{
				Status:     "error",
				Error:      resp.Error.Error(),
				DurationMs: resp.Duration.Milliseconds(),
			})
			return
		}

		// Success - convert scaling.Result to RunResponse
		writeJSON(w, http.StatusOK, RunResponse{
			Status:     resp.Result.Status,
			Output:     resp.Result.Output,
			RawOutput:  fmt.Sprintf("%v", resp.Result.Output), // Best effort string conversion
			Error:      resp.Result.Error,
			Stderr:     resp.Result.Stderr,
			DurationMs: resp.Duration.Milliseconds(),
		})

	case <-r.Context().Done():
		writeError(w, http.StatusRequestTimeout, "request timeout")
	}
}

// executeDirectly executes an agent directly without worker pool (fallback).
func (s *Server) executeDirectly(w http.ResponseWriter, r *http.Request, agent *registry.RegisteredAgent) {
	// Parse request (simplified - no agent_path needed!)
	var req struct {
		Input json.RawMessage   `json:"input"`
		Env   map[string]string `json:"env,omitempty"` // Optional runtime overrides
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Generate request ID for execlog tracking
	requestID := uuid.New().String()

	// Helper to log execlog events (best-effort, non-blocking for QUEUED)
	logExecEvent := func(state string, durationMs *int64, errMsg *string) {
		writer, err := execlog.NewWriter(s.execlogDir, agent.Name)
		if err != nil {
			log.Printf("Warning: Failed to create execlog writer: %v", err)
			return
		}
		event := &execlog.Event{
			Timestamp:  time.Now(),
			RequestID:  requestID,
			State:      state,
			DurationMs: durationMs,
			Error:      errMsg,
		}
		if err := writer.Log(event); err != nil {
			log.Printf("Warning: Failed to log %s: %v", state, err)
		}
	}

	// Log QUEUED state (async, best-effort)
	go logExecEvent(execlog.StateQueued, nil, nil)

	// Build full RunRequest using stored agent metadata
	fullReq := &RunRequest{
		AgentPath: agent.Path,
		Input:     make(map[string]interface{}),
		Env:       make(map[string]string),
	}

	// Parse input
	if len(req.Input) > 0 {
		json.Unmarshal(req.Input, &fullReq.Input)
	}

	// Start with stored env vars (from deploy)
	for _, envStr := range agent.ResolvedEnv {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) == 2 {
			fullReq.Env[parts[0]] = parts[1]
		}
	}

	// Apply runtime overrides (if any)
	for k, v := range req.Env {
		fullReq.Env[k] = v
	}

	// Log STARTED state and track execution time
	logExecEvent(execlog.StateStarted, nil, nil)
	startTime := time.Now()

	// Execute using existing flow (pass ServiceManager for model server management)
	ctx := r.Context()
	result, err := Execute(ctx, fullReq, s.serviceManager)

	// Calculate duration
	durationMs := time.Since(startTime).Milliseconds()

	// Log COMPLETED or FAILED state
	// Check both err != nil AND result.Status != success to catch OOM/timeout
	if err != nil || result.Status != proxy.StatusSuccess {
		// Log FAILED state
		var errStr string
		if err != nil {
			errStr = err.Error()
		} else if result.Error != "" {
			errStr = result.Error
		}
		logExecEvent(execlog.StateFailed, &durationMs, &errStr)

		writeJSON(w, http.StatusOK, RunResponse{
			Status:     "error",
			Error:      errStr,
			DurationMs: durationMs,
		})
		return
	}

	// Log COMPLETED state
	logExecEvent(execlog.StateCompleted, &durationMs, nil)

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

// executeViaPoolStreaming executes an agent request via worker pool with SSE streaming.
func (s *Server) executeViaPoolStreaming(w http.ResponseWriter, r *http.Request, agentName string, pool *AgentPool) {
	// Parse request
	var req struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Extract session ID from header for session affinity (Phase 2)
	sessionID := r.Header.Get(pool.sessionConfig.Key)

	// Create SSE writer
	sseWriter := runtime.NewSSEWriter(w)
	if sseWriter == nil {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	defer sseWriter.Close()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send init event
	sseWriter.WriteEvent(&runtime.StreamEvent{
		Type:      "init",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"agent_name": agentName,
		},
	})

	// Create streaming request with StreamCh
	streamCh := make(chan *scaling.StreamEvent, 100) // Buffered to prevent blocking
	scalingReq := &scaling.Request{
		ID:         uuid.New().String(),
		Input:      req.Input,
		Context:    r.Context(),
		ResponseCh: make(chan *scaling.Response, 1),
		StreamCh:   streamCh,    // Enable streaming
		SessionID:  sessionID,   // Session affinity (Phase 2)
	}

	// Log QUEUED state (best-effort, async)
	go func() {
		writer, err := execlog.NewWriter(s.execlogDir, agentName)
		if err != nil {
			log.Printf("Warning: Failed to create execlog writer: %v", err)
			return
		}
		err = writer.Log(&execlog.Event{
			Timestamp: time.Now(),
			RequestID: scalingReq.ID,
			State:     execlog.StateQueued,
			SessionID: ptrOrNil(scalingReq.SessionID),
		})
		if err != nil {
			log.Printf("Warning: Failed to log QUEUED: %v", err)
		}
	}()

	// Enqueue request
	if err := pool.queue.Enqueue(r.Context(), scalingReq); err != nil {
		writeError(w, http.StatusServiceUnavailable, "queue full")
		return
	}

	// Stream events until channel closes or context done
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := range streamCh {
			// Convert scaling.StreamEvent → runtime.StreamEvent
			runtimeEvent := &runtime.StreamEvent{
				Type:      event.Type,
				Timestamp: event.Timestamp,
				Data:      event.Data,
			}
			if err := sseWriter.WriteEvent(runtimeEvent); err != nil {
				// Client disconnected - context will cancel
				return
			}
		}
	}()

	// Wait for completion or context cancellation
	select {
	case resp := <-scalingReq.ResponseCh:
		// Wait for all events to stream
		<-done

		// Send final completed event
		status := "success"
		var output interface{}
		if resp.Error != nil {
			status = "error"
		} else if resp.Result != nil {
			status = resp.Result.Status
			output = resp.Result.Output
		}

		sseWriter.WriteEvent(&runtime.StreamEvent{
			Type:      "completed",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"status":      status,
				"duration_ms": resp.Duration.Milliseconds(),
				"output":      output,
			},
		})

	case <-r.Context().Done():
		// Client disconnected - cleanup happens via defer
		return
	}
}

// handleRunByNameStreaming handles SSE streaming execution.
func (s *Server) handleRunByNameStreaming(w http.ResponseWriter, r *http.Request, agent *registry.RegisteredAgent) {
	// Parse request (simplified - no agent_path needed!)
	var req struct {
		Input   json.RawMessage   `json:"input"`
		Env     map[string]string `json:"env,omitempty"` // Optional runtime overrides
		Options struct {
			Timeout int `json:"timeout,omitempty"` // Timeout in seconds
		} `json:"options,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		return
	}

	// Generate request ID for execlog tracking
	requestID := uuid.New().String()

	// Helper to log execlog events (best-effort)
	logExecEvent := func(state string, durationMs *int64, errMsg *string) {
		writer, err := execlog.NewWriter(s.execlogDir, agent.Name)
		if err != nil {
			log.Printf("Warning: Failed to create execlog writer: %v", err)
			return
		}
		event := &execlog.Event{
			Timestamp:  time.Now(),
			RequestID:  requestID,
			State:      state,
			DurationMs: durationMs,
			Error:      errMsg,
		}
		if err := writer.Log(event); err != nil {
			log.Printf("Warning: Failed to log %s: %v", state, err)
		}
	}

	// Log QUEUED state (async, best-effort)
	go logExecEvent(execlog.StateQueued, nil, nil)

	// Create SSE writer and verify streaming is supported
	sseWriter := runtime.NewSSEWriter(w)
	if sseWriter == nil {
		writeError(w, http.StatusInternalServerError, "streaming not supported by this connection")
		return
	}
	defer sseWriter.Close()

	// Set SSE headers (after writer validation)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Generate agent ID (for running agent tracking, separate from request ID)
	agentID := fmt.Sprintf("agent-%s", uuid.New().String()[:8])

	// Send init event
	sseWriter.WriteEvent(&runtime.StreamEvent{
		Type:      "init",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"agent_id":   agentID,
			"agent_name": agent.Name,
		},
	})

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
		AgentPath: agent.Path,
		StartedAt: time.Now(),
		Cancel:    cancel,
	}
	s.registerAgent(runningAgent)
	defer s.unregisterAgent(agentID)

	// Build full RunRequest using stored agent metadata
	fullReq := &RunRequest{
		AgentPath: agent.Path,
		Input:     make(map[string]interface{}),
		Env:       make(map[string]string),
	}

	// Parse input
	if len(req.Input) > 0 {
		json.Unmarshal(req.Input, &fullReq.Input)
	}

	// Start with stored env vars (from deploy)
	for _, envStr := range agent.ResolvedEnv {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) == 2 {
			fullReq.Env[parts[0]] = parts[1]
		}
	}

	// Apply runtime overrides (if any)
	for k, v := range req.Env {
		fullReq.Env[k] = v
	}

	// Log STARTED state and track execution time
	logExecEvent(execlog.StateStarted, nil, nil)
	startTime := time.Now()

	// Execute agent with streaming (pass ServiceManager for model server management)
	result, err := ExecuteStreaming(ctx, fullReq, sseWriter, s.serviceManager)

	// Calculate duration
	durationMs := time.Since(startTime).Milliseconds()

	// Send error event if execution failed
	// Check both err != nil AND result.Status != success to catch OOM/timeout
	if err != nil || result.Status != proxy.StatusSuccess {
		// Log FAILED state
		var errStr string
		if err != nil {
			errStr = err.Error()
		} else if result.Error != "" {
			errStr = result.Error
		}
		logExecEvent(execlog.StateFailed, &durationMs, &errStr)

		sseWriter.WriteEvent(&runtime.StreamEvent{
			Type:      "error",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"error":       errStr,
				"duration_ms": time.Since(runningAgent.StartedAt).Milliseconds(),
			},
		})
		sseWriter.WriteEvent(&runtime.StreamEvent{
			Type:      "completed",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"status":      "error",
				"duration_ms": time.Since(runningAgent.StartedAt).Milliseconds(),
			},
		})
		return
	}

	// Log COMPLETED state
	logExecEvent(execlog.StateCompleted, &durationMs, nil)

	// Send completed event with final result
	sseWriter.WriteEvent(&runtime.StreamEvent{
		Type:      "completed",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"status":      string(result.Status),
			"duration_ms": result.Duration.Milliseconds(),
			"output":      result.Output,
			"error":       result.Error,
		},
	})
}

// handleAgentsList lists all registered agents.
// GET /v1/agents
func (s *Server) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agents, err := s.registry.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list agents: %v", err))
		return
	}

	// Return agent list
	response := map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetAgentByName returns details for a specific agent.
// GET /v1/agents/{name}
func (s *Server) handleGetAgentByName(w http.ResponseWriter, agentName string) {
	agent, err := s.registry.Get(agentName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	// Return agent details (without exposing full env var values)
	response := map[string]interface{}{
		"name":       agent.Name,
		"created_at": agent.CreatedAt,
		"updated_at": agent.UpdatedAt,
		"env_vars":   extractEnvKeys(agent.ResolvedEnv), // Just keys, not values
	}

	writeJSON(w, http.StatusOK, response)
}

// handleDeleteAgentByName unregisters an agent and removes its files.
// DELETE /v1/agents/{name}
func (s *Server) handleDeleteAgentByName(w http.ResponseWriter, agentName string) {
	// NEW Phase 2: Remove pool first (before registry)
	if s.poolManager != nil {
		if poolErr := s.poolManager.RemovePool(agentName); poolErr != nil {
			log.Printf("[handler] Failed to remove pool for '%s': %v", agentName, poolErr)
			// Continue - pool might not exist (agent deployed before Phase 2)
		}
	}

	// Get agent path before deleting from registry
	agent, err := s.registry.Get(agentName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	// Delete from registry
	if err := s.registry.Delete(agentName); err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	// Delete agent directory from disk
	// Agent path points to /agent subdir, so get parent for full rootfs
	agentDir := filepath.Dir(agent.Path) // .../agents/{name}/agent -> .../agents/{name}
	if agentDir != "" && agentDir != "/" && agentDir != "." {
		if removeErr := os.RemoveAll(agentDir); removeErr != nil {
			log.Printf("[handler] Warning: Failed to remove agent directory '%s': %v", agentDir, removeErr)
			// Continue - registry already cleaned up
		} else {
			log.Printf("[handler] Removed agent directory: %s", agentDir)
		}
	}

	// Delete workspace directory
	workspaceBaseDir := "/var/lib/orpheus/workspaces"
	if _, statErr := os.Stat("/var/lib/orpheus"); os.IsNotExist(statErr) {
		home, _ := os.UserHomeDir()
		workspaceBaseDir = filepath.Join(home, ".orpheus", "workspaces")
	}
	workspaceDir := filepath.Join(workspaceBaseDir, agentName)
	if workspaceDir != "" && workspaceDir != "/" && workspaceDir != "." {
		if removeErr := os.RemoveAll(workspaceDir); removeErr != nil {
			log.Printf("[handler] Warning: Failed to remove workspace '%s': %v", workspaceDir, removeErr)
			// Continue - agent already cleaned up
		} else {
			log.Printf("[handler] Removed workspace directory: %s", workspaceDir)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "agent undeployed",
		"name":    agentName,
	})
}

// handleAgentLogs returns agent execution logs (stub for now).
// GET /v1/agents/{name}/logs
func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request, agentName string) {
	// Verify agent exists
	if _, err := s.registry.Get(agentName); err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	// TODO: Implement log streaming
	writeError(w, http.StatusNotImplemented, "agent logs not yet implemented")
}

// extractEnvKeys extracts just the keys from env var strings (not values).
func extractEnvKeys(envVars []string) []string {
	keys := make([]string, 0, len(envVars))
	for _, envStr := range envVars {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) > 0 {
			keys = append(keys, parts[0])
		}
	}
	return keys
}

// ============================================================================
// Stats Endpoint (Phase 3)
// ============================================================================

// AgentStatsResponse represents statistics for a single agent.
type AgentStatsResponse struct {
	AgentName string    `json:"agent_name"`
	CreatedAt time.Time `json:"created_at"`

	Queue   *QueueStatsResponse `json:"queue,omitempty"`
	Pool    *PoolStatsResponse  `json:"pool,omitempty"`
	Derived *DerivedMetrics     `json:"derived,omitempty"`

	// For agents without pools (backward compatibility)
	PoolStatus string `json:"pool_status,omitempty"`
	Message    string `json:"message,omitempty"`
}

// QueueStatsResponse represents queue metrics.
type QueueStatsResponse struct {
	Pending        int     `json:"pending"`
	Processing     int     `json:"processing"`
	Total          int     `json:"total"`
	MaxSize        int     `json:"max_size"`
	IsClosed       bool    `json:"is_closed"`
	FillPercentage float64 `json:"fill_percentage"`
}

// PoolStatsResponse represents worker pool metrics.
type PoolStatsResponse struct {
	TotalWorkers  int       `json:"total_workers"`
	IdleWorkers   int       `json:"idle_workers"`
	BusyWorkers   int       `json:"busy_workers"`
	DesiredSize   int       `json:"desired_size"`
	LastScaleTime time.Time `json:"last_scale_time,omitempty"`
}

// DerivedMetrics represents computed metrics.
type DerivedMetrics struct {
	UtilizationPercentage float64 `json:"utilization_percentage"`
	RequestsPerWorker     float64 `json:"requests_per_worker"`
	PoolEfficiency        string  `json:"pool_efficiency"` // "low", "medium", "high"
}

// AllStatsResponse represents stats for all agents.
type AllStatsResponse struct {
	Agents    []AgentStatsResponse `json:"agents"`
	Global    GlobalStatsResponse  `json:"global"`
	Timestamp time.Time            `json:"timestamp"`
}

// GlobalStatsResponse represents aggregated stats across all agents.
type GlobalStatsResponse struct {
	TotalAgents        int     `json:"total_agents"`
	TotalWorkers       int     `json:"total_workers"`
	TotalPending       int     `json:"total_pending"`
	TotalProcessing    int     `json:"total_processing"`
	TotalQueueSize     int     `json:"total_queue_size"`
	AverageUtilization float64 `json:"average_utilization"`
	AgentsWithPools    int     `json:"agents_with_pools"`
	AgentsWithoutPools int     `json:"agents_without_pools"`
}

// handleStats returns statistics for all agents or a specific agent.
// GET /v1/stats?agent=<name> - Specific agent
// GET /v1/stats - All agents
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check for agent query parameter
	agentName := r.URL.Query().Get("agent")
	if agentName != "" {
		// Single agent stats
		s.handleAgentStatsQuery(w, r, agentName)
		return
	}

	// All agents stats
	s.handleAllStats(w, r)
}

// handleAgentStatsQuery handles stats for a specific agent via query parameter.
func (s *Server) handleAgentStatsQuery(w http.ResponseWriter, r *http.Request, agentName string) {
	stats, err := s.collectAgentStats(agentName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleAgentStats returns statistics for a specific agent.
// GET /v1/agents/{name}/stats
func (s *Server) handleAgentStats(w http.ResponseWriter, r *http.Request, agentName string) {
	stats, err := s.collectAgentStats(agentName)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleAllStats returns statistics for all agents with global aggregation.
func (s *Server) handleAllStats(w http.ResponseWriter, r *http.Request) {
	// Get all agents from registry
	agents, err := s.registry.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list agents: %v", err))
		return
	}

	// Collect stats for each agent
	agentStats := make([]AgentStatsResponse, 0, len(agents))
	global := GlobalStatsResponse{}

	for _, agent := range agents {
		stats, err := s.collectAgentStats(agent.Name)
		if err != nil {
			continue // Skip agents that error
		}

		agentStats = append(agentStats, *stats)

		// Aggregate global stats
		global.TotalAgents++

		if stats.Pool != nil {
			global.TotalWorkers += stats.Pool.TotalWorkers
			global.AgentsWithPools++
		} else {
			global.AgentsWithoutPools++
		}

		if stats.Queue != nil {
			global.TotalPending += stats.Queue.Pending
			global.TotalProcessing += stats.Queue.Processing
			global.TotalQueueSize += stats.Queue.Total
		}
	}

	// Calculate average utilization
	if global.AgentsWithPools > 0 {
		totalUtil := 0.0
		count := 0
		for _, stats := range agentStats {
			if stats.Derived != nil {
				totalUtil += stats.Derived.UtilizationPercentage
				count++
			}
		}
		if count > 0 {
			global.AverageUtilization = totalUtil / float64(count)
		}
	}

	response := AllStatsResponse{
		Agents:    agentStats,
		Global:    global,
		Timestamp: time.Now(),
	}

	writeJSON(w, http.StatusOK, response)
}

// collectAgentStats collects statistics for a single agent.
func (s *Server) collectAgentStats(agentName string) (*AgentStatsResponse, error) {
	// Get agent from registry
	agent, err := s.registry.Get(agentName)
	if err != nil {
		return nil, err
	}

	stats := &AgentStatsResponse{
		AgentName: agentName,
		CreatedAt: agent.CreatedAt,
	}

	// Try to get pool
	if s.poolManager != nil {
		pool, poolErr := s.poolManager.GetPool(agentName)
		if poolErr == nil {
			// Pool exists - collect metrics
			stats.Queue = s.collectQueueStats(pool)
			stats.Pool = s.collectPoolStats(pool)
			stats.Derived = computeDerivedMetrics(stats.Queue, stats.Pool)
		} else {
			// No pool - agent deployed before Phase 2
			stats.PoolStatus = "not_available"
			stats.Message = "Agent deployed before Phase 2 autoscaling. Undeploy and redeploy to enable worker pools."
		}
	} else {
		// PoolManager disabled
		stats.PoolStatus = "disabled"
		stats.Message = "Autoscaling not enabled"
	}

	return stats, nil
}

// collectQueueStats collects queue metrics from an agent pool.
func (s *Server) collectQueueStats(pool *AgentPool) *QueueStatsResponse {
	qStats := pool.queue.GetStats()

	fillPercentage := 0.0
	if qStats.MaxSize > 0 {
		total := qStats.PendingCount + qStats.ProcessingCount
		fillPercentage = (float64(total) / float64(qStats.MaxSize)) * 100.0
	}

	return &QueueStatsResponse{
		Pending:        qStats.PendingCount,
		Processing:     qStats.ProcessingCount,
		Total:          qStats.PendingCount + qStats.ProcessingCount,
		MaxSize:        qStats.MaxSize,
		IsClosed:       qStats.IsClosed,
		FillPercentage: fillPercentage,
	}
}

// collectPoolStats collects worker pool metrics from an agent pool.
func (s *Server) collectPoolStats(pool *AgentPool) *PoolStatsResponse {
	pStats := pool.pool.GetStats()

	return &PoolStatsResponse{
		TotalWorkers:  pStats.TotalWorkers,
		IdleWorkers:   pStats.IdleWorkers,
		BusyWorkers:   pStats.BusyWorkers,
		DesiredSize:   pStats.DesiredSize,
		LastScaleTime: pStats.LastScaleTime,
	}
}

// computeDerivedMetrics calculates derived metrics from queue and pool stats.
func computeDerivedMetrics(queueStats *QueueStatsResponse, poolStats *PoolStatsResponse) *DerivedMetrics {
	if queueStats == nil || poolStats == nil {
		return nil
	}

	totalRequests := queueStats.Pending + queueStats.Processing
	totalWorkers := poolStats.TotalWorkers

	var utilization float64
	if totalWorkers > 0 {
		utilization = (float64(totalRequests) / float64(totalWorkers)) * 100.0
	}

	var requestsPerWorker float64
	if totalWorkers > 0 {
		requestsPerWorker = float64(totalRequests) / float64(totalWorkers)
	}

	efficiency := "low"
	if utilization >= 70 {
		efficiency = "high"
	} else if utilization >= 40 {
		efficiency = "medium"
	}

	return &DerivedMetrics{
		UtilizationPercentage: utilization,
		RequestsPerWorker:     requestsPerWorker,
		PoolEfficiency:        efficiency,
	}
}

// ============================================================================
// Workspace Endpoints
// ============================================================================

// WorkspaceInfoResponse represents workspace information for an agent.
type WorkspaceInfoResponse struct {
	AgentName string           `json:"agent_name"`
	Path      string           `json:"path"`
	SizeBytes int64            `json:"size_bytes"`
	FileCount int              `json:"file_count"`
	Files     map[string]int64 `json:"files,omitempty"` // filename → size
	Exists    bool             `json:"exists"`
}

// WorkspaceCleanResponse represents the result of cleaning a workspace.
type WorkspaceCleanResponse struct {
	Status     string `json:"status"`
	AgentName  string `json:"agent_name"`
	FreedBytes int64  `json:"freed_bytes"`
}

// resolveWorkspacePathByName returns the workspace directory path for an agent by name.
func resolveWorkspacePathByName(agentName string) string {
	workspaceBaseDir := "/var/lib/orpheus/workspaces"
	if _, statErr := os.Stat("/var/lib/orpheus"); os.IsNotExist(statErr) {
		home, _ := os.UserHomeDir()
		workspaceBaseDir = filepath.Join(home, ".orpheus", "workspaces")
	}
	return filepath.Join(workspaceBaseDir, agentName)
}

// getDirectoryInfo calculates the total size and file count of a directory.
func getDirectoryInfo(path string) (sizeBytes int64, fileCount int, files map[string]int64, err error) {
	files = make(map[string]int64)

	err = filepath.Walk(path, func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			sizeBytes += info.Size()
			fileCount++
			// Store relative path
			relPath, _ := filepath.Rel(path, filePath)
			files[relPath] = info.Size()
		}
		return nil
	})

	return sizeBytes, fileCount, files, err
}

// handleWorkspaceInfo returns workspace information for an agent.
// GET /v1/agents/{name}/workspace
func (s *Server) handleWorkspaceInfo(w http.ResponseWriter, r *http.Request, agentName string) {
	// Verify agent exists
	if _, err := s.registry.Get(agentName); err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	workspacePath := resolveWorkspacePathByName(agentName)

	response := WorkspaceInfoResponse{
		AgentName: agentName,
		Path:      workspacePath,
		Exists:    false,
		Files:     make(map[string]int64),
	}

	// Check if workspace exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		// Workspace doesn't exist yet - return empty response
		writeJSON(w, http.StatusOK, response)
		return
	}

	response.Exists = true

	// Get directory info
	sizeBytes, fileCount, files, err := getDirectoryInfo(workspacePath)
	if err != nil {
		log.Printf("[workspace] Error reading workspace '%s': %v", workspacePath, err)
		// Still return what we have
	}

	response.SizeBytes = sizeBytes
	response.FileCount = fileCount
	response.Files = files

	writeJSON(w, http.StatusOK, response)
}

// handleWorkspaceClean removes workspace contents (keeps directory).
// DELETE /v1/agents/{name}/workspace
func (s *Server) handleWorkspaceClean(w http.ResponseWriter, r *http.Request, agentName string) {
	// Verify agent exists
	if _, err := s.registry.Get(agentName); err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	workspacePath := resolveWorkspacePathByName(agentName)

	// Check if workspace exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		// Workspace doesn't exist - nothing to clean
		writeJSON(w, http.StatusOK, WorkspaceCleanResponse{
			Status:     "success",
			AgentName:  agentName,
			FreedBytes: 0,
		})
		return
	}

	// Calculate size before cleanup
	sizeBytes, _, _, _ := getDirectoryInfo(workspacePath)

	// Remove all contents but keep the directory
	entries, err := os.ReadDir(workspacePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read workspace: %v", err))
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(workspacePath, entry.Name())
		if err := os.RemoveAll(entryPath); err != nil {
			log.Printf("[workspace] Warning: Failed to remove '%s': %v", entryPath, err)
		}
	}

	log.Printf("[workspace] Cleaned workspace for '%s': freed %d bytes", agentName, sizeBytes)

	writeJSON(w, http.StatusOK, WorkspaceCleanResponse{
		Status:     "success",
		AgentName:  agentName,
		FreedBytes: sizeBytes,
	})
}

// ptrOrNil returns a pointer to the string, or nil if empty
func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// handleExecLogCrashed returns all crashed requests across all agents
// GET /v1/execlog/crashed
func (s *Server) handleExecLogCrashed(w http.ResponseWriter, r *http.Request) {
	// Only support GET
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Get all deployed agents
	agents, err := s.registry.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list agents: %v", err))
		return
	}

	// Query each agent's execlog for crashed requests
	var allCrashed []map[string]interface{}

	for _, agent := range agents {
		reader, err := execlog.NewReader(s.execlogDir, agent.Name)
		if err != nil {
			// Skip if execlog doesn't exist for this agent
			continue
		}

		crashed, err := reader.GetCrashedRequests()
		reader.Close()

		if err != nil {
			log.Printf("Warning: Failed to read crashed for %s: %v", agent.Name, err)
			continue
		}

		// Convert to response format
		for _, req := range crashed {
			crashedMap := map[string]interface{}{
				"request_id":  req.RequestID,
				"agent_name":  agent.Name,
				"worker_id":   req.WorkerID,
				"started_at":  req.StartedAt.Format(time.RFC3339),
			}
			if req.SessionID != nil {
				crashedMap["session_id"] = *req.SessionID
			}
			allCrashed = append(allCrashed, crashedMap)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"crashed_requests": allCrashed,
		"count":            len(allCrashed),
	})
}

// handleExecLog returns filtered and paginated execution logs
// GET /v1/execlog?agent=xxx&status=xxx&session=xxx&worker=xxx&limit=50&offset=0
func (s *Server) handleExecLog(w http.ResponseWriter, r *http.Request) {
	// Only support GET
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse query parameters
	agentFilter := r.URL.Query().Get("agent")
	statusFilter := r.URL.Query().Get("status")
	sessionFilter := r.URL.Query().Get("session")
	workerFilter := r.URL.Query().Get("worker")

	// Parse pagination (default limit: 50, max: 1000)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Get all deployed agents
	agents, err := s.registry.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list agents: %v", err))
		return
	}

	// Filter agents if specified
	var filteredAgents []registry.RegisteredAgent
	if agentFilter != "" {
		for _, agent := range agents {
			if agent.Name == agentFilter {
				filteredAgents = append(filteredAgents, agent)
				break
			}
		}
	} else {
		filteredAgents = agents
	}

	// Query each agent's execlog
	var allLogs []map[string]interface{}
	totalCount := 0

	for _, agent := range filteredAgents {
		reader, err := execlog.NewReader(s.execlogDir, agent.Name)
		if err != nil {
			continue // Skip if no execlog for this agent
		}

		// Build filters
		filters := &execlog.ExecLogFilters{
			Status:    statusFilter,
			WorkerID:  workerFilter,
			SessionID: sessionFilter,
			Limit:     limit,
			Offset:    offset,
		}

		// Get logs
		logs, err := reader.GetExecutionLogs(filters)
		if err != nil {
			log.Printf("Warning: Failed to query logs for %s: %v", agent.Name, err)
			reader.Close()
			continue
		}

		// Get total count
		count, err := reader.GetExecutionLogsCount(filters)
		reader.Close()

		if err != nil {
			log.Printf("Warning: Failed to count logs for %s: %v", agent.Name, err)
		} else {
			totalCount += count
		}

		// Convert to response format
		for _, entry := range logs {
			logMap := map[string]interface{}{
				"request_id": entry.RequestID,
				"agent_name": agent.Name,
				"state":      entry.State,
				"timestamp":  time.Unix(0, entry.Timestamp).Format(time.RFC3339),
			}
			if entry.WorkerID != nil {
				logMap["worker_id"] = *entry.WorkerID
			}
			if entry.SessionID != nil {
				logMap["session_id"] = *entry.SessionID
			}
			if entry.DurationMs != nil {
				logMap["duration_ms"] = *entry.DurationMs
			}
			if entry.Error != nil {
				logMap["error"] = *entry.Error
			}

			allLogs = append(allLogs, logMap)
		}
	}

	// Calculate pagination metadata
	page := (offset / limit) + 1
	totalPages := 0
	if totalCount > 0 {
		totalPages = (totalCount + limit - 1) / limit
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":        allLogs,
		"count":       len(allLogs),
		"total":       totalCount,
		"page":        page,
		"limit":       limit,
		"offset":      offset,
		"total_pages": totalPages,
	})
}

// handleExecLogStats returns aggregated execution statistics
// GET /v1/execlog/stats?agent=<name>
func (s *Server) handleExecLogStats(w http.ResponseWriter, r *http.Request) {
	// Only support GET
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentFilter := r.URL.Query().Get("agent")

	// Get all deployed agents
	agents, err := s.registry.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list agents: %v", err))
		return
	}

	// Filter agents if specified
	var targetAgents []registry.RegisteredAgent
	if agentFilter != "" {
		for _, agent := range agents {
			if agent.Name == agentFilter {
				targetAgents = append(targetAgents, agent)
				break
			}
		}
	} else {
		targetAgents = agents
	}

	// Collect stats from each agent
	var agentStats []map[string]interface{}
	globalTotal := 0
	globalCompleted := 0
	globalFailed := 0
	globalCrashed := 0
	globalAvgDuration := 0.0
	agentCount := 0

	for _, agent := range targetAgents {
		reader, err := execlog.NewReader(s.execlogDir, agent.Name)
		if err != nil {
			continue // Skip agents without execlog
		}

		stats, err := reader.GetStats()
		reader.Close()

		if err != nil {
			log.Printf("Warning: Failed to get stats for %s: %v", agent.Name, err)
			continue
		}

		// Aggregate global
		globalTotal += stats.Total
		globalCompleted += stats.Completed
		globalFailed += stats.Failed
		globalCrashed += stats.Crashed
		globalAvgDuration += stats.AvgDuration
		agentCount++

		// Add to response
		agentStats = append(agentStats, map[string]interface{}{
			"agent_name":      agent.Name,
			"total":           stats.Total,
			"completed":       stats.Completed,
			"failed":          stats.Failed,
			"crashed":         stats.Crashed,
			"success_rate":    stats.SuccessRate,
			"avg_duration_ms": stats.AvgDuration,
			"health_status":   stats.HealthStatus,
		})
	}

	// Calculate global metrics
	globalSuccessRate := 0.0
	if globalTotal > 0 {
		globalSuccessRate = float64(globalCompleted) / float64(globalTotal) * 100.0
	}
	if agentCount > 0 {
		globalAvgDuration = globalAvgDuration / float64(agentCount)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"agents": agentStats,
		"global": map[string]interface{}{
			"total_requests":  globalTotal,
			"completed":       globalCompleted,
			"failed":          globalFailed,
			"crashed":         globalCrashed,
			"success_rate":    globalSuccessRate,
			"avg_duration_ms": globalAvgDuration,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
