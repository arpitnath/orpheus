package daemon

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"agentscale/pkg/deploy"
)

// DeployResponse is the response for POST /v1/deploy.
type DeployResponse struct {
	AgentName  string            `json:"agent_name"`
	Status     string            `json:"status"`
	Endpoints  map[string]string `json:"endpoints"`
	SizeMB     int               `json:"size_mb"`
	DeployedAt string            `json:"deployed_at"`
}

// handleDeploy handles POST /v1/deploy for remote agent deployment.
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse multipart form (max 2GB upload)
	if err := r.ParseMultipartForm(2 << 30); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("parse form: %v", err))
		return
	}

	// Get agent name
	agentName := r.FormValue("agent_name")
	if agentName == "" {
		writeError(w, http.StatusBadRequest, "agent_name is required")
		return
	}

	// Validate agent name (security)
	if err := deploy.ValidateAgentName(agentName); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid agent_name: %v", err))
		return
	}

	// Get checksum
	expectedChecksum := r.FormValue("checksum")
	if expectedChecksum == "" {
		writeError(w, http.StatusBadRequest, "checksum is required")
		return
	}

	// Get uploaded tar file
	file, fileHeader, err := r.FormFile("agent_tar")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("agent_tar file required: %v", err))
		return
	}
	defer file.Close()

	log.Printf("Received deployment for agent '%s' (%d MB)", agentName, fileHeader.Size/(1024*1024))

	// Save uploaded file to temp location
	tempFile, err := os.CreateTemp("", "agentscale-deploy-*.tar.gz")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create temp file: %v", err))
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath) // Cleanup temp file

	// Copy uploaded file to temp
	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save upload: %v", err))
		return
	}
	tempFile.Close()

	// Verify checksum
	if err := deploy.VerifyChecksum(tempPath, expectedChecksum); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("checksum verification failed: %v", err))
		return
	}

	log.Printf("Checksum verified for agent '%s'", agentName)

	// Determine agent directory
	// Use /var/lib/agentscale/agents/ if it exists, else ~/.agentscale/agents/
	agentBaseDir := "/var/lib/agentscale/agents"
	if _, err := os.Stat("/var/lib/agentscale"); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		agentBaseDir = filepath.Join(home, ".agentscale", "agents")
	}

	agentDir := filepath.Join(agentBaseDir, agentName)

	// Check if agent already exists
	if _, err := os.Stat(agentDir); err == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("agent '%s' already exists (use undeploy first)", agentName))
		return
	}

	// Ensure base directory exists
	if err := os.MkdirAll(agentBaseDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create base dir: %v", err))
		return
	}

	// Open tar file for extraction
	tarFile, err := os.Open(tempPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("open tar: %v", err))
		return
	}
	defer tarFile.Close()

	log.Printf("Starting extraction to %s...", agentBaseDir)

	// Extract tar to agents directory
	// Extract to parent directory, tar contains {agentName}/ prefix
	if err := deploy.ExtractTar(tarFile, agentBaseDir); err != nil {
		// Cleanup partial extraction
		os.RemoveAll(agentDir)
		log.Printf("ERROR: Extraction failed: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("extract tar: %v", err))
		return
	}

	log.Printf("Extracted agent '%s' to %s", agentName, agentDir)

	// Verify agent.yaml exists
	agentYAML := filepath.Join(agentDir, "agent.yaml")
	if _, err := os.Stat(agentYAML); os.IsNotExist(err) {
		os.RemoveAll(agentDir)
		writeError(w, http.StatusBadRequest, "agent.yaml not found in uploaded tar")
		return
	}

	// TODO: Load agent.yaml and validate
	// TODO: Register with autoscaler (if multi-agent mode)

	// Calculate deployed size
	sizeMB := calculateDirSizeMB(agentDir)

	// Build endpoint URLs
	// For now, return basic endpoints (will be enhanced with MCP in Phase 4)
	serverDomain := r.Host // Use request host for now
	endpoints := map[string]string{
		"http": fmt.Sprintf("http://%s/v1/agents/run?agent=%s", serverDomain, agentName),
	}

	// Return success response
	response := DeployResponse{
		AgentName:  agentName,
		Status:     "deployed",
		Endpoints:  endpoints,
		SizeMB:     sizeMB,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, response)
	log.Printf("Agent '%s' deployed successfully (%d MB)", agentName, sizeMB)
}

// calculateDirSizeMB calculates directory size in megabytes.
func calculateDirSizeMB(dirPath string) int {
	var size int64
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return int(size / (1024 * 1024))
}
