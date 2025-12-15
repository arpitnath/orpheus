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

// handleInvoke processes POST /invoke requests.
// It enqueues the request and waits for a worker to process it.
func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	// Enqueue the request
	if err := s.queue.Enqueue(r.Context(), req); err != nil {
		if errors.Is(err, scaling.ErrQueueFull) {
			writeError(w, http.StatusServiceUnavailable, "queue is full, try again later")
			return
		}
		if errors.Is(err, scaling.ErrQueueClosed) {
			writeError(w, http.StatusServiceUnavailable, "server is shutting down")
			return
		}
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	log.Printf("[handler] Enqueued request %s", req.ID)

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

// HealthResponse is the JSON response for /health.
type HealthResponse struct {
	Status  string `json:"status"`
	AgentID string `json:"agent_id"`
	Tier    string `json:"tier"`
}

// handleHealth returns the server's health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "healthy",
		AgentID: s.cfg.Name,
		Tier:    s.tier,
	})
}

// StatsResponse is the JSON response for /stats.
type StatsResponse struct {
	AgentID string `json:"agent_id"`
	Tier    string `json:"tier"`

	// Queue stats
	QueuePending    int  `json:"queue_pending"`
	QueueProcessing int  `json:"queue_processing"`
	QueueTotal      int  `json:"queue_total"`
	QueueClosed     bool `json:"queue_closed"`

	// Pool stats
	PoolTotal   int `json:"pool_total"`
	PoolDesired int `json:"pool_desired"`
}

// handleStats returns queue and pool statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	queueStats := s.queue.GetStats()
	poolStats := s.pool.GetStats()

	writeJSON(w, http.StatusOK, StatsResponse{
		AgentID:         s.cfg.Name,
		Tier:            s.tier,
		QueuePending:    queueStats.PendingCount,
		QueueProcessing: queueStats.ProcessingCount,
		QueueTotal:      queueStats.PendingCount + queueStats.ProcessingCount,
		QueueClosed:     queueStats.IsClosed,
		PoolTotal:       poolStats.TotalWorkers,
		PoolDesired:     poolStats.DesiredSize,
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
