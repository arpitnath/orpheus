package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"agentscale/pkg/scaling"
)

// requestCounter generates unique request IDs.
var requestCounter atomic.Uint64

// generateID creates a unique request ID.
func generateID() string {
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), requestCounter.Add(1))
}

// extractAgentID gets agent ID from query parameter.
func extractAgentID(r *http.Request) (string, error) {
	agentID := r.URL.Query().Get("agent")
	if agentID == "" {
		return "", errors.New("missing required query parameter: agent")
	}
	return agentID, nil
}

// InvokeRequest is the expected JSON body for /invoke.
type InvokeRequest struct {
	Input json.RawMessage `json:"input"`
}

// InvokeResponse is the JSON response for /invoke.
type InvokeResponse struct {
	Status     string         `json:"status"`
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
}

// handleInvoke processes POST /invoke?agent=<agent_id> requests.
// It routes the request to the specified agent's queue and waits for response.
func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract agent ID from query parameter
	agentID, err := extractAgentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get agent instance
	instance, err := s.getAgentInstance(agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Read and parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Parse JSON - accept either {"input": ...} or raw JSON
	var input []byte
	var invokeReq InvokeRequest
	if err := json.Unmarshal(body, &invokeReq); err == nil && invokeReq.Input != nil {
		input = invokeReq.Input
	} else {
		// Accept raw JSON as input
		input = body
	}

	// Create request with response channel
	req := &scaling.Request{
		ID:         generateID(),
		Input:      input,
		Context:    r.Context(),
		ResponseCh: make(chan *scaling.Response, 1),
	}

	// Enqueue to agent's queue
	if err := instance.queue.Enqueue(r.Context(), req); err != nil {
		if errors.Is(err, scaling.ErrQueueFull) {
			writeError(w, http.StatusServiceUnavailable,
				fmt.Sprintf("queue is full for agent %s, try again later", agentID))
			return
		}
		if errors.Is(err, scaling.ErrQueueClosed) {
			writeError(w, http.StatusServiceUnavailable, "server is shutting down")
			return
		}
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	log.Printf("[handler] Enqueued request %s to agent '%s'", req.ID, agentID)

	// Wait for response
	select {
	case resp := <-req.ResponseCh:
		if resp.Error != nil {
			writeJSON(w, http.StatusInternalServerError, InvokeResponse{
				Status:     "error",
				Error:      resp.Error.Error(),
				DurationMs: resp.Duration.Milliseconds(),
			})
			return
		}

		// Success response
		writeJSON(w, http.StatusOK, InvokeResponse{
			Status:     resp.Result.Status,
			Output:     resp.Result.Output,
			DurationMs: resp.Duration.Milliseconds(),
		})

	case <-r.Context().Done():
		// Client disconnected or request timeout
		writeError(w, http.StatusGatewayTimeout, "request timeout")
	}
}

// AgentHealthInfo represents health info for one agent.
type AgentHealthInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Workers int    `json:"workers"`
}

// HealthResponse is the JSON response for /health.
type HealthResponse struct {
	Status string            `json:"status"`
	Agents []AgentHealthInfo `json:"agents"`
}

// handleHealth returns the server's health status and lists all agents.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]AgentHealthInfo, 0, len(s.instances))
	for agentID, instance := range s.instances {
		agents = append(agents, AgentHealthInfo{
			ID:      agentID,
			Name:    instance.cfg.Name,
			Workers: instance.pool.Size(),
		})
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status: "healthy",
		Agents: agents,
	})
}

// StatsResponse is the JSON response for /stats.
type StatsResponse struct {
	AgentID string `json:"agent_id"`

	// Queue stats
	QueuePending    int  `json:"queue_pending"`
	QueueProcessing int  `json:"queue_processing"`
	QueueTotal      int  `json:"queue_total"`
	QueueClosed     bool `json:"queue_closed"`

	// Pool stats
	PoolTotal   int `json:"pool_total"`
	PoolDesired int `json:"pool_desired"`
}

// AllStatsResponse is the JSON response for /stats with no agent specified.
type AllStatsResponse struct {
	Agents []StatsResponse `json:"agents"`
	Global GlobalStats     `json:"global"`
}

// GlobalStats represents aggregate stats across all agents.
type GlobalStats struct {
	TotalWorkers    int `json:"total_workers"`
	TotalPending    int `json:"total_pending"`
	TotalProcessing int `json:"total_processing"`
}

// handleStats returns queue and pool statistics.
// GET /stats?agent=<agent_id> - Stats for specific agent
// GET /stats - Stats for all agents
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	agentID := r.URL.Query().Get("agent")

	if agentID != "" {
		// Stats for specific agent
		instance, err := s.getAgentInstance(agentID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}

		queueStats := instance.queue.GetStats()
		poolStats := instance.pool.GetStats()

		writeJSON(w, http.StatusOK, StatsResponse{
			AgentID:         agentID,
			QueuePending:    queueStats.PendingCount,
			QueueProcessing: queueStats.ProcessingCount,
			QueueTotal:      queueStats.PendingCount + queueStats.ProcessingCount,
			QueueClosed:     queueStats.IsClosed,
			PoolTotal:       poolStats.TotalWorkers,
			PoolDesired:     poolStats.DesiredSize,
		})
		return
	}

	// All agents stats
	s.mu.RLock()
	defer s.mu.RUnlock()

	allStats := make([]StatsResponse, 0, len(s.instances))
	totalWorkers := 0
	totalPending := 0
	totalProcessing := 0

	for agentID, instance := range s.instances {
		queueStats := instance.queue.GetStats()
		poolStats := instance.pool.GetStats()

		allStats = append(allStats, StatsResponse{
			AgentID:         agentID,
			QueuePending:    queueStats.PendingCount,
			QueueProcessing: queueStats.ProcessingCount,
			QueueTotal:      queueStats.PendingCount + queueStats.ProcessingCount,
			QueueClosed:     queueStats.IsClosed,
			PoolTotal:       poolStats.TotalWorkers,
			PoolDesired:     poolStats.DesiredSize,
		})

		totalWorkers += poolStats.TotalWorkers
		totalPending += queueStats.PendingCount
		totalProcessing += queueStats.ProcessingCount
	}

	writeJSON(w, http.StatusOK, AllStatsResponse{
		Agents: allStats,
		Global: GlobalStats{
			TotalWorkers:    totalWorkers,
			TotalPending:    totalPending,
			TotalProcessing: totalProcessing,
		},
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[handler] Error encoding JSON response: %v", err)
	}
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"status": "error",
		"error":  message,
	})
}
