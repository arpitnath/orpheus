package execlog

import "time"

// States for request lifecycle tracking
const (
	StateQueued    = "QUEUED"
	StateStarted   = "STARTED"
	StateCompleted = "COMPLETED"
	StateFailed    = "FAILED"
	StateCrashed   = "CRASHED"
)

// Event represents a single state transition in the request lifecycle
type Event struct {
	Timestamp  time.Time
	RequestID  string
	State      string
	WorkerID   *string // Null for QUEUED
	SessionID  *string // Null if no session affinity
	DurationMs *int64  // Only for COMPLETED/FAILED
	Error      *string // Only for FAILED
}

// CrashedRequest represents a request that was executing when daemon crashed
type CrashedRequest struct {
	RequestID  string
	AgentName  string
	WorkerID   string
	SessionID  *string
	StartedAt  time.Time
}

// ExecLogFilters defines filtering and pagination parameters
type ExecLogFilters struct {
	Status    string
	WorkerID  string
	SessionID string
	StartTime int64 // Unix nano
	EndTime   int64 // Unix nano
	Limit     int
	Offset    int
}

// ExecLogEntry represents a single execution log entry
type ExecLogEntry struct {
	RequestID  string
	State      string
	WorkerID   *string
	SessionID  *string
	Timestamp  int64
	DurationMs *int64
	Error      *string
}

// ExecLogStats holds aggregated statistics for an agent's execution logs
type ExecLogStats struct {
	// Counts by state
	Queued    int
	Started   int
	Completed int
	Failed    int
	Crashed   int
	Total     int

	// Duration metrics (milliseconds)
	AvgDuration float64
	MinDuration int64
	MaxDuration int64

	// Derived metrics
	SuccessRate  float64 // completed/total * 100
	ErrorRate    float64 // (failed+crashed)/total * 100
	CrashRate    float64 // crashed/total * 100
	HealthStatus string  // "healthy"/"degraded"/"unhealthy"
}
