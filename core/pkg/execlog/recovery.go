package execlog

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// DetectAndMarkCrashed finds crashed requests across all agents and marks them as CRASHED
// Called on daemon startup to recover from crashes
func DetectAndMarkCrashed(execlogDir string) (map[string][]*CrashedRequest, error) {
	// Find all .db files (one per agent)
	pattern := filepath.Join(execlogDir, "*.db")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob execlog files: %w", err)
	}

	allCrashed := make(map[string][]*CrashedRequest)

	for _, dbFile := range files {
		// Extract agent name from filename
		base := filepath.Base(dbFile)
		agentName := strings.TrimSuffix(base, ".db")

		// Skip SQLite internal files
		if strings.HasSuffix(agentName, "-wal") || strings.HasSuffix(agentName, "-shm") {
			continue
		}

		// Read crashed requests for this agent
		reader, err := NewReader(execlogDir, agentName)
		if err != nil {
			log.Printf("Warning: Failed to open execlog for %s: %v", agentName, err)
			continue
		}

		crashed, err := reader.GetCrashedRequests()
		reader.Close()

		if err != nil {
			log.Printf("Warning: Failed to query crashed for %s: %v", agentName, err)
			continue
		}

		if len(crashed) == 0 {
			continue
		}

		// Mark each as CRASHED
		writer, err := NewWriter(execlogDir, agentName)
		if err != nil {
			log.Printf("Warning: Failed to open writer for %s: %v", agentName, err)
			continue
		}

		for _, req := range crashed {
			event := &Event{
				Timestamp: time.Now(),
				RequestID: req.RequestID,
				State:     StateCrashed,
				WorkerID:  &req.WorkerID,
				SessionID: req.SessionID,
			}

			if err := writer.Log(event); err != nil {
				log.Printf("Warning: Failed to mark %s as crashed: %v", req.RequestID, err)
			}

			// Set agent name for response
			req.AgentName = agentName
		}

		// NOTE: Don't close the writer - it's cached and will be reused by other code
		// The writer is designed to persist across the daemon's lifetime

		allCrashed[agentName] = crashed
	}

	return allCrashed, nil
}
