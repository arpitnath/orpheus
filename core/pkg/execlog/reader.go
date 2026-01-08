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

// GetCrashedRequests finds requests that were STARTED but never reached a terminal state
func (r *Reader) GetCrashedRequests() ([]*CrashedRequest, error) {
	rows, err := r.db.Query(`
		SELECT request_id, worker_id, session_id, MIN(timestamp) as started_at
		FROM events
		WHERE state = ?
		  AND request_id NOT IN (
			  SELECT request_id FROM events
			  WHERE state IN (?, ?, ?)
		  )
		GROUP BY request_id
		ORDER BY timestamp DESC
	`, StateStarted, StateCompleted, StateFailed, StateCrashed)

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

// Close closes the database connection
func (r *Reader) Close() error {
	return r.db.Close()
}
