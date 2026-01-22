package execlog

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Reader handles querying execution log events from SQLite
type Reader struct {
	db *sql.DB
}

// NewReader creates a new ExecLog reader for the specified agent
func NewReader(execlogDir, agentName string) (*Reader, error) {
	dbPath := filepath.Join(execlogDir, agentName+".db")

	// Open SQLite database in read-only mode
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &Reader{db: db}, nil
}

// GetCrashedRequests returns all requests that have STARTED but no terminal state (crashed)
// These are requests that were interrupted by a daemon crash
func (r *Reader) GetCrashedRequests() ([]*CrashedRequest, error) {
	// Find requests with STARTED state that lack a terminal state (COMPLETED, FAILED, CRASHED)
	rows, err := r.db.Query(`
		SELECT DISTINCT e1.request_id, e1.worker_id, e1.session_id, e1.timestamp as started_at
		FROM events e1
		LEFT JOIN events e2 ON e1.request_id = e2.request_id
		                    AND e2.state IN (?, ?, ?)
		WHERE e1.state = ?
		  AND e2.request_id IS NULL
		ORDER BY e1.timestamp DESC
	`, StateCompleted, StateFailed, StateCrashed, StateStarted)

	if err != nil {
		return nil, fmt.Errorf("query crashed requests: %w", err)
	}
	defer rows.Close()

	var crashed []*CrashedRequest
	for rows.Next() {
		var req CrashedRequest
		var startedAtNano int64
		var workerID, sessionID sql.NullString

		err := rows.Scan(&req.RequestID, &workerID, &sessionID, &startedAtNano)
		if err != nil {
			continue // Skip malformed rows
		}

		req.WorkerID = workerID.String
		if sessionID.Valid {
			req.SessionID = &sessionID.String
		}
		req.StartedAt = time.Unix(0, startedAtNano)

		crashed = append(crashed, &req)
	}

	return crashed, nil
}

// GetExecutionLogs returns filtered and paginated execution logs
func (r *Reader) GetExecutionLogs(filters *ExecLogFilters) ([]*ExecLogEntry, error) {
	query := `SELECT request_id, state, worker_id, session_id, timestamp, duration_ms, error, source, mcp_caller
	          FROM events WHERE 1=1`
	args := []interface{}{}

	// Add filters dynamically
	if filters.Status != "" {
		query += ` AND state = ?`
		args = append(args, filters.Status)
	}
	if filters.WorkerID != "" {
		query += ` AND worker_id = ?`
		args = append(args, filters.WorkerID)
	}
	if filters.SessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, filters.SessionID)
	}
	if filters.Source != "" {
		query += ` AND source = ?`
		args = append(args, filters.Source)
	}
	if filters.StartTime > 0 {
		query += ` AND timestamp >= ?`
		args = append(args, filters.StartTime)
	}
	if filters.EndTime > 0 {
		query += ` AND timestamp <= ?`
		args = append(args, filters.EndTime)
	}

	// Sorting and pagination
	query += ` ORDER BY timestamp DESC`
	if filters.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, filters.Limit, filters.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution logs: %w", err)
	}
	defer rows.Close()

	var entries []*ExecLogEntry
	for rows.Next() {
		var entry ExecLogEntry
		var workerID, sessionID sql.NullString
		var durationMs sql.NullInt64
		var errorMsg sql.NullString
		var source, mcpCaller sql.NullString

		err := rows.Scan(&entry.RequestID, &entry.State, &workerID, &sessionID,
			&entry.Timestamp, &durationMs, &errorMsg, &source, &mcpCaller)
		if err != nil {
			continue // Skip malformed rows
		}

		if workerID.Valid {
			entry.WorkerID = &workerID.String
		}
		if sessionID.Valid {
			entry.SessionID = &sessionID.String
		}
		if durationMs.Valid {
			entry.DurationMs = &durationMs.Int64
		}
		if errorMsg.Valid {
			entry.Error = &errorMsg.String
		}
		if source.Valid {
			entry.Source = &source.String
		}
		if mcpCaller.Valid {
			entry.MCPCaller = &mcpCaller.String
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// GetExecutionLogsCount returns the total count of logs matching the filters
func (r *Reader) GetExecutionLogsCount(filters *ExecLogFilters) (int, error) {
	query := `SELECT COUNT(*) FROM events WHERE 1=1`
	args := []interface{}{}

	// Add same filters (without LIMIT/OFFSET)
	if filters.Status != "" {
		query += ` AND state = ?`
		args = append(args, filters.Status)
	}
	if filters.WorkerID != "" {
		query += ` AND worker_id = ?`
		args = append(args, filters.WorkerID)
	}
	if filters.SessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, filters.SessionID)
	}
	if filters.Source != "" {
		query += ` AND source = ?`
		args = append(args, filters.Source)
	}
	if filters.StartTime > 0 {
		query += ` AND timestamp >= ?`
		args = append(args, filters.StartTime)
	}
	if filters.EndTime > 0 {
		query += ` AND timestamp <= ?`
		args = append(args, filters.EndTime)
	}

	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// GetStats returns aggregated execution statistics for this agent
func (r *Reader) GetStats() (*ExecLogStats, error) {
	stats := &ExecLogStats{}

	// Count by state
	rows, err := r.db.Query(`
		SELECT state, COUNT(*) as count
		FROM events
		GROUP BY state
	`)
	if err != nil {
		return nil, fmt.Errorf("query state counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			continue
		}

		switch state {
		case StateQueued:
			stats.Queued = count
		case StateStarted:
			stats.Started = count
		case StateCompleted:
			stats.Completed = count
		case StateFailed:
			stats.Failed = count
		case StateCrashed:
			stats.Crashed = count
		}
		stats.Total += count
	}

	// Duration stats (for COMPLETED requests only)
	var avgDuration, minDuration, maxDuration sql.NullFloat64
	err = r.db.QueryRow(`
		SELECT AVG(duration_ms), MIN(duration_ms), MAX(duration_ms)
		FROM events
		WHERE state = ? AND duration_ms IS NOT NULL
	`, StateCompleted).Scan(&avgDuration, &minDuration, &maxDuration)

	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("query duration stats: %w", err)
	}

	if avgDuration.Valid {
		stats.AvgDuration = avgDuration.Float64
	}
	if minDuration.Valid {
		stats.MinDuration = int64(minDuration.Float64)
	}
	if maxDuration.Valid {
		stats.MaxDuration = int64(maxDuration.Float64)
	}

	// Calculate derived metrics
	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.Completed) / float64(stats.Total) * 100.0
		stats.ErrorRate = float64(stats.Failed+stats.Crashed) / float64(stats.Total) * 100.0
		stats.CrashRate = float64(stats.Crashed) / float64(stats.Total) * 100.0

		// Determine health status
		if stats.ErrorRate < 1.0 && stats.CrashRate == 0 {
			stats.HealthStatus = "healthy"
		} else if stats.ErrorRate < 5.0 && stats.CrashRate < 0.5 {
			stats.HealthStatus = "degraded"
		} else {
			stats.HealthStatus = "unhealthy"
		}
	} else {
		stats.HealthStatus = "no_data"
	}

	return stats, nil
}

// Close closes the database connection
func (r *Reader) Close() error {
	return r.db.Close()
}
