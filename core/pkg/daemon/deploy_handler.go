package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"orpheus/daemon/pkg/config"
	"orpheus/daemon/pkg/deploy"
	"orpheus/daemon/pkg/registry"
)

// DeployResponse is the response for POST /v1/deploy.
type DeployResponse struct {
	AgentName    string            `json:"agent_name"`
	Status       string            `json:"status"`
	Endpoints    map[string]string `json:"endpoints"`
	SizeMB       int               `json:"size_mb"`
	DeployedAt   string            `json:"deployed_at"`
	Dependencies *DependencyInfo   `json:"dependencies,omitempty"`
}

// DependencyInfo contains information about installed dependencies.
type DependencyInfo struct {
	Installed bool   `json:"installed"`
	Runtime   string `json:"runtime"`
	Source    string `json:"source,omitempty"` // requirements.txt, package.json
}

// DeployProgressEvent is sent during SSE streaming deploys.
type DeployProgressEvent struct {
	Phase    string `json:"phase"`
	Message  string `json:"message"`
	Progress int    `json:"progress"`
}

// handleDeploy handles POST /v1/deploy for remote agent deployment.
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check for SSE streaming request
	useSSE := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var flusher http.Flusher
	if useSSE {
		var ok bool
		flusher, ok = w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "SSE not supported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
	}

	// Helper to emit SSE events
	emit := func(event, phase, msg string, progress int) {
		if !useSSE {
			return
		}
		data, _ := json.Marshal(DeployProgressEvent{
			Phase:    phase,
			Message:  msg,
			Progress: progress,
		})
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	// Helper to emit error events
	emitError := func(phase, errMsg string) {
		if !useSSE {
			return
		}
		data, _ := json.Marshal(map[string]string{
			"phase": phase,
			"error": errMsg,
		})
		fmt.Fprintf(w, "event: deploy_error\ndata: %s\n\n", data)
		flusher.Flush()
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
	tempFile, err := os.CreateTemp("", "orpheus-deploy-*.tar.gz")
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
	emit("deploy_progress", "extracting", "Extracting agent tarball...", 20)

	// Determine agent directory
	// Use /var/lib/orpheus/agents/ if it exists, else ~/.orpheus/agents/
	agentBaseDir := "/var/lib/orpheus/agents"
	if _, err := os.Stat("/var/lib/orpheus"); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		agentBaseDir = filepath.Join(home, ".orpheus", "agents")
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

	// Extract to temp directory first to read agent.yaml
	tempExtractDir, err := os.MkdirTemp("", "orpheus-extract-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create temp extract dir: %v", err))
		return
	}
	defer os.RemoveAll(tempExtractDir)

	// Open tar file for extraction
	tarFile, err := os.Open(tempPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("open tar: %v", err))
		return
	}
	defer tarFile.Close()

	log.Printf("Extracting to temp directory...")

	// Extract tar to temp directory
	if err := deploy.ExtractTar(tarFile, tempExtractDir); err != nil {
		log.Printf("ERROR: Extraction failed: %v", err)
		if useSSE {
			emitError("extracting", fmt.Sprintf("extract tar: %v", err))
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("extract tar: %v", err))
		}
		return
	}

	// Agent code is in tempExtractDir/{agentName}/
	tempAgentDir := filepath.Join(tempExtractDir, agentName)

	// Verify agent.yaml exists
	agentYAML := filepath.Join(tempAgentDir, "agent.yaml")
	if _, err := os.Stat(agentYAML); os.IsNotExist(err) {
		if useSSE {
			emitError("validating", "agent.yaml not found in uploaded tar")
		} else {
			writeError(w, http.StatusBadRequest, "agent.yaml not found in uploaded tar")
		}
		return
	}

	// Load agent.yaml to get runtime
	agentConfig, err := config.Load(tempAgentDir)
	if err != nil {
		if useSSE {
			emitError("validating", fmt.Sprintf("invalid agent.yaml: %v", err))
		} else {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid agent.yaml: %v", err))
		}
		return
	}

	log.Printf("Agent runtime: %s", agentConfig.Runtime)
	emit("deploy_progress", "validating", "Agent configuration valid", 30)

	// Determine base image path based on runtime
	baseImagePath, err := resolveBaseImagePath(agentConfig.Runtime)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("resolve base image: %v", err))
		return
	}

	log.Printf("Using base image: %s", baseImagePath)

	// Copy base image to agent directory
	log.Printf("Copying base image to %s...", agentDir)
	if err := copyDir(baseImagePath, agentDir); err != nil {
		os.RemoveAll(agentDir)
		if useSSE {
			emitError("copying", fmt.Sprintf("copy base image: %v", err))
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("copy base image: %v", err))
		}
		return
	}

	// Copy agent code into /agent subdirectory
	agentCodeDir := filepath.Join(agentDir, "agent")
	log.Printf("Copying agent code to %s...", agentCodeDir)
	if err := copyDir(tempAgentDir, agentCodeDir); err != nil {
		os.RemoveAll(agentDir)
		if useSSE {
			emitError("copying", fmt.Sprintf("copy agent code: %v", err))
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("copy agent code: %v", err))
		}
		return
	}
	emit("deploy_progress", "copying", "Base image and agent code ready", 50)

	// Create workspace directory for persistent storage
	workspaceDir, err := createWorkspaceDir(agentName)
	if err != nil {
		os.RemoveAll(agentDir)
		if useSSE {
			emitError("workspace", fmt.Sprintf("create workspace: %v", err))
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("create workspace: %v", err))
		}
		return
	}
	log.Printf("Created workspace directory: %s", workspaceDir)
	emit("deploy_progress", "workspace", "Workspace directory created", 55)

	// Install dependencies based on runtime
	log.Printf("Installing dependencies for '%s' (runtime: %s)...", agentName, agentConfig.Runtime)
	depInfo, err := installDependencies(agentConfig.Runtime, agentCodeDir, agentDir)
	if err != nil {
		os.RemoveAll(agentDir)
		if useSSE {
			emitError("installing", fmt.Sprintf("install dependencies: %v", err))
		} else {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("install dependencies: %v", err))
		}
		return
	}
	if depInfo != nil && depInfo.Installed {
		log.Printf("Dependencies installed for '%s' from %s", agentName, depInfo.Source)
		emit("deploy_progress", "installing", fmt.Sprintf("Dependencies installed from %s", depInfo.Source), 70)
	} else {
		log.Printf("No dependencies to install for '%s'", agentName)
		emit("deploy_progress", "installing", "No dependencies required", 70)
	}

	log.Printf("Deployed agent '%s' with base image merge", agentName)

	// Calculate deployed size
	sizeMB := calculateDirSizeMB(agentDir)

	// Extract org_id from API key (for MCP endpoint)
	orgID := getOrgIDFromRequest(r)

	// Parse form data to get resolved env vars (sent from CLI)
	var resolvedEnv []string
	if envData := r.FormValue("env"); envData != "" {
		if unmarshalErr := json.Unmarshal([]byte(envData), &resolvedEnv); unmarshalErr != nil {
			log.Printf("Warning: Failed to parse env data: %v", unmarshalErr)
			// Continue without env vars
		}
	}

	// Register agent in registry
	// Path points to agent code directory (agentDir/agent), not rootfs
	// resolveImagePath() finds the rootfs separately based on agent name
	if s.registry != nil {
		regErr := s.registry.Register(registry.RegisteredAgent{
			Name:        agentName,
			Runtime:     agentConfig.Runtime,
			Path:        agentCodeDir, // Points to /agent subdirectory for config loading
			ResolvedEnv: resolvedEnv,
		})
		if regErr != nil {
			log.Printf("Warning: Failed to register agent in registry: %v", regErr)
			// Continue anyway - registration is optional for now
		} else {
			log.Printf("Agent '%s' registered in registry", agentName)
		}
	}

	// Create autoscaling pool for agent (NEW - Phase 2)
	if s.poolManager != nil {
		poolErr := s.poolManager.CreatePool(agentName)
		if poolErr != nil {
			log.Printf("Warning: Failed to create pool for '%s': %v", agentName, poolErr)
			log.Printf("Agent will fall back to direct execution mode")
			// Continue - agent can still work without pool (fallback to direct execution)
		} else {
			log.Printf("Created autoscaling pool for agent '%s'", agentName)
		}
	}
	emit("deploy_progress", "registering", "Agent registered in pool", 90)

	// Build endpoint URLs (NEW RESTful format)
	serverDomain := r.Host // Use request host for now
	endpoints := map[string]string{
		"http": fmt.Sprintf("http://%s/v1/agents/%s/run", serverDomain, agentName),
		"mcp":  fmt.Sprintf("mcp://%s/mcp/%s/agents/%s", serverDomain, orgID, agentName),
	}

	// Build success response
	response := DeployResponse{
		AgentName:    agentName,
		Status:       "deployed",
		Endpoints:    endpoints,
		SizeMB:       sizeMB,
		DeployedAt:   time.Now().UTC().Format(time.RFC3339),
		Dependencies: depInfo,
	}

	// Return response based on mode
	if useSSE {
		// Emit deploy_complete event with full response
		data, _ := json.Marshal(response)
		fmt.Fprintf(w, "event: deploy_complete\ndata: %s\n\n", data)
		flusher.Flush()
	} else {
		writeJSON(w, http.StatusOK, response)
	}
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

// getOrgIDFromRequest extracts org_id from the API key in the request.
// For v0.1.0, derives org_id from API key hash.
// Future: Get org_id from API key database lookup.
func getOrgIDFromRequest(r *http.Request) string {
	// Extract API key from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "org-unknown"
	}

	// Parse "Bearer <key>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "org-unknown"
	}

	apiKey := strings.TrimSpace(parts[1])

	// Derive org_id from API key hash (v0.1.0 simple approach)
	// Hash the key and take first 16 hex chars
	hash := sha256.Sum256([]byte(apiKey))
	orgID := "org-" + hex.EncodeToString(hash[:])[:12]

	return orgID
}

// resolveBaseImagePath returns the path to the base image for the given runtime.
// Images are stored in ~/.orpheus/images/{imageName}/
func resolveBaseImagePath(runtime string) (string, error) {
	var imageName string
	switch runtime {
	case config.RuntimeNodeJS20:
		imageName = "nodejs-20"
	case config.RuntimePython3, "":
		imageName = "python-3.10"
	default:
		return "", fmt.Errorf("unsupported runtime: %s", runtime)
	}

	// Try multiple possible locations for base images
	// 1. /var/lib/orpheus/images (system-wide)
	// 2. ~/.orpheus/images (user)
	// 3. For Lima: mounted macOS home directory
	searchPaths := []string{}

	// System path
	searchPaths = append(searchPaths, filepath.Join("/var/lib/orpheus/images", imageName))

	// User home path
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".orpheus", "images", imageName))
	}

	// Lima mounted macOS home (try common paths)
	macOSHomes := []string{"/Users/arpit", "/home/arpit"}
	for _, macHome := range macOSHomes {
		searchPaths = append(searchPaths, filepath.Join(macHome, ".orpheus", "images", imageName))
	}

	// Find first existing path with /lib directory (valid rootfs)
	for _, path := range searchPaths {
		libPath := filepath.Join(path, "lib")
		if _, err := os.Stat(libPath); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("base image '%s' not found (tried: %v)", imageName, searchPaths)
}

// copyDir copies a directory recursively using cp -a for efficiency.
func copyDir(src, dst string) error {
	// Ensure destination exists
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("create dst dir: %w", err)
	}

	// Use cp -a for efficient recursive copy with permissions preserved
	cmd := exec.Command("cp", "-a", src+"/.", dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cp failed: %v: %s", err, string(output))
	}
	return nil
}

// installDependencies installs agent dependencies based on runtime.
// - Python: pip install -r requirements.txt --target /packages
// - Node.js: npm install in agent code directory
// Returns DependencyInfo describing what was installed.
func installDependencies(runtime, agentCodeDir, agentDir string) (*DependencyInfo, error) {
	switch runtime {
	case config.RuntimePython3, "":
		installed, err := installPythonDeps(agentCodeDir, agentDir)
		if err != nil {
			return nil, err
		}
		if installed {
			return &DependencyInfo{Installed: true, Runtime: "python3", Source: "requirements.txt"}, nil
		}
		return &DependencyInfo{Installed: false, Runtime: "python3"}, nil
	case config.RuntimeNodeJS20:
		installed, err := installNodeDeps(agentCodeDir)
		if err != nil {
			return nil, err
		}
		if installed {
			return &DependencyInfo{Installed: true, Runtime: "nodejs20", Source: "package.json"}, nil
		}
		return &DependencyInfo{Installed: false, Runtime: "nodejs20"}, nil
	default:
		return nil, nil // Unknown runtime, skip
	}
}

// installPythonDeps installs Python dependencies from requirements.txt.
// Returns true if dependencies were installed, false if skipped.
func installPythonDeps(agentCodeDir, agentDir string) (bool, error) {
	requirementsFile := filepath.Join(agentCodeDir, "requirements.txt")
	if _, err := os.Stat(requirementsFile); os.IsNotExist(err) {
		log.Printf("No requirements.txt found, skipping Python dependency install")
		return false, nil // No requirements.txt, skip
	}

	packagesDir := filepath.Join(agentDir, "packages")
	log.Printf("Installing Python dependencies to %s", packagesDir)

	cmd := exec.Command("pip3", "install",
		"-r", requirementsFile,
		"--target", packagesDir,
		"--quiet",
		"--no-cache-dir")

	if output, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("pip install failed: %v: %s", err, string(output))
	}
	return true, nil
}

// installNodeDeps installs Node.js dependencies from package.json.
// Returns true if dependencies were installed, false if skipped.
func installNodeDeps(agentCodeDir string) (bool, error) {
	packageJSON := filepath.Join(agentCodeDir, "package.json")
	if _, err := os.Stat(packageJSON); os.IsNotExist(err) {
		log.Printf("No package.json found, skipping Node.js dependency install")
		return false, nil // No package.json, skip
	}

	log.Printf("Installing Node.js dependencies in %s", agentCodeDir)

	cmd := exec.Command("npm", "install", "--prefix", agentCodeDir, "--quiet")
	if output, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("npm install failed: %v: %s", err, string(output))
	}
	return true, nil
}

// createWorkspaceDir creates the persistent workspace directory for an agent.
// Returns the workspace path or error.
// The workspace is used for persistent storage that survives container restarts.
func createWorkspaceDir(agentName string) (string, error) {
	// Determine workspace base directory (mirrors agent base dir logic)
	workspaceBaseDir := "/var/lib/orpheus/workspaces"
	if _, err := os.Stat("/var/lib/orpheus"); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		workspaceBaseDir = filepath.Join(home, ".orpheus", "workspaces")
	}

	workspaceDir := filepath.Join(workspaceBaseDir, agentName)

	// Create workspace directory
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}

	// Change ownership to container UID (1000:1000)
	// This ensures the non-root container process can write to it
	if err := os.Chown(workspaceDir, 1000, 1000); err != nil {
		// Log warning but don't fail - might work anyway if daemon runs as root
		// or if the directory is already writable by UID 1000
		log.Printf("Warning: Failed to chown workspace to 1000:1000: %v", err)
	}

	return workspaceDir, nil
}
