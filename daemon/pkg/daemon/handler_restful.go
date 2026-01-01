package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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

	// Create scaling request
	scalingReq := &scaling.Request{
		ID:         uuid.New().String(),
		Input:      req.Input,
		Context:    r.Context(),
		ResponseCh: make(chan *scaling.Response, 1),
	}

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

	// Execute using existing flow
	ctx := r.Context()
	result, err := Execute(ctx, fullReq)
	if err != nil {
		writeJSON(w, http.StatusOK, RunResponse{
			Status:     "error",
			Error:      err.Error(),
			DurationMs: 0,
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
		StreamCh:   streamCh, // Enable streaming
	}

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

	// Generate agent ID
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

	// Execute agent with streaming
	result, err := ExecuteStreaming(ctx, fullReq, sseWriter)

	// Send error event if execution failed
	if err != nil {
		sseWriter.WriteEvent(&runtime.StreamEvent{
			Type:      "error",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"error":       err.Error(),
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

// handleDeleteAgentByName unregisters an agent.
// DELETE /v1/agents/{name}
func (s *Server) handleDeleteAgentByName(w http.ResponseWriter, agentName string) {
	// NEW Phase 2: Remove pool first (before registry)
	if s.poolManager != nil {
		if poolErr := s.poolManager.RemovePool(agentName); poolErr != nil {
			log.Printf("[handler] Failed to remove pool for '%s': %v", agentName, poolErr)
			// Continue - pool might not exist (agent deployed before Phase 2)
		}
	}

	// Delete from registry
	if err := s.registry.Delete(agentName); err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("agent not found: %s", agentName))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "agent unregistered",
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
