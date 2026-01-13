package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"orpheus/daemon/pkg/config"
	"orpheus/daemon/pkg/generator"
	"orpheus/daemon/pkg/proxy"
	"orpheus/daemon/pkg/runtime"
	"orpheus/daemon/pkg/service"
)

// Execute runs an agent using the existing proxy.RunAgent infrastructure.
// This is the bridge between the daemon API and the execution engine.
// serviceManager is optional - pass nil for agents that don't need model servers.
func Execute(ctx context.Context, req *RunRequest, serviceManager *service.Manager) (*proxy.Result, error) {
	// Load agent config
	cfg, err := config.Load(req.AgentPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// If agent specifies a model, use ServiceManager to ensure model server is running
	if cfg.Model != "" {
		// Check if user already set OPENAI_BASE_URL (user's env takes priority)
		hasBaseURL := false
		hasAPIKey := false
		for _, env := range cfg.Env {
			if len(env) > 16 && env[:16] == "OPENAI_BASE_URL=" {
				hasBaseURL = true
			}
			if len(env) > 15 && env[:15] == "OPENAI_API_KEY=" {
				hasAPIKey = true
			}
		}

		// Only inject if not user-provided
		if !hasBaseURL {
			// ServiceManager is required for model server management
			if serviceManager == nil {
				return nil, fmt.Errorf("agent requires model '%s' but ServiceManager is not available", cfg.Model)
			}

			modelConfig := service.ModelConfig{
				Name:   cfg.Model,
				Server: "auto", // Auto-detect backend (Ollama on Mac, vLLM on Linux+GPU)
			}
			modelEndpoint, err := serviceManager.EnsureModelServer(ctx, modelConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to ensure model server for '%s': %w", cfg.Model, err)
			}

			log.Printf("[executor] ServiceManager provided endpoint: %s", modelEndpoint)
			cfg.Env = append(cfg.Env,
				"MODEL_URL="+modelEndpoint,
				"OPENAI_BASE_URL="+modelEndpoint+"/v1",
			)
		}
		if !hasAPIKey {
			cfg.Env = append(cfg.Env,
				"OPENAI_API_KEY=orpheus-internal-key",
			)
		}

		log.Printf("[executor] Agent uses model '%s'", cfg.Model)
	}

	// Merge runtime environment variable overrides (after auto-injection)
	if len(req.Env) > 0 {
		for k, v := range req.Env {
			cfg.Env = append(cfg.Env, k+"="+v)
		}
	}

	// Generate entrypoint
	gen := generator.New()
	entrypointPath, err := gen.Generate(cfg, false) // sync execution
	if err != nil {
		return nil, fmt.Errorf("generate entrypoint: %w", err)
	}
	defer gen.Cleanup(entrypointPath)

	// Resolve image path (deployed agent directory)
	// For now, use agent_path directly as the rootfs
	// In production, this would come from the deploy command's image registry
	imagePath := resolveImagePath(req.AgentPath)

	// Build execute options
	opts := &proxy.ExecuteOptions{
		Input:         marshalInput(req.Input),
		UseIsolate:    true,
		RootFSPath:    imagePath,
		WorkspacePath: resolveWorkspacePath(req.AgentPath),
	}

	// Apply memory limits
	if req.Options.MemoryLimit > 0 {
		opts.MemoryLimit = req.Options.MemoryLimit
		opts.MemoryTarget = req.Options.MemoryLimit / 2 // Target is half of limit
		opts.SwapEnabled = true
	}

	// Apply timeout options
	if req.Options.IdleTimeout > 0 {
		opts.IdleTimeout = time.Duration(req.Options.IdleTimeout) * time.Second
	}
	if req.Options.Timeout > 0 {
		opts.MaxTimeout = time.Duration(req.Options.Timeout) * time.Second
	}

	// Activity check interval defaults to 5 seconds
	opts.ActivityCheck = 5 * time.Second

	// Execute agent using existing infrastructure
	result := proxy.RunAgent(ctx, cfg, entrypointPath, opts)
	return result, nil
}

// ExecuteStreaming runs an agent with real-time SSE output streaming.
// Similar to Execute() but passes streamWriter for real-time event emission.
// serviceManager is optional - pass nil for agents that don't need model servers.
func ExecuteStreaming(ctx context.Context, req *RunRequest, streamWriter runtime.StreamWriter, serviceManager *service.Manager) (*proxy.Result, error) {
	// Load agent config
	cfg, err := config.Load(req.AgentPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Merge runtime environment variable overrides
	if len(req.Env) > 0 {
		for k, v := range req.Env {
			cfg.Env = append(cfg.Env, k+"="+v)
		}
	}

	// If agent specifies a model, use ServiceManager to ensure model server is running
	if cfg.Model != "" {
		// Check if user already set OPENAI_BASE_URL (user's env takes priority)
		hasBaseURL := false
		hasAPIKey := false
		for _, env := range cfg.Env {
			if len(env) > 16 && env[:16] == "OPENAI_BASE_URL=" {
				hasBaseURL = true
			}
			if len(env) > 15 && env[:15] == "OPENAI_API_KEY=" {
				hasAPIKey = true
			}
		}

		// Only inject if not user-provided
		if !hasBaseURL {
			// ServiceManager is required for model server management
			if serviceManager == nil {
				return nil, fmt.Errorf("agent requires model '%s' but ServiceManager is not available", cfg.Model)
			}

			modelConfig := service.ModelConfig{
				Name:   cfg.Model,
				Server: "auto", // Auto-detect backend
			}
			modelEndpoint, err := serviceManager.EnsureModelServer(ctx, modelConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to ensure model server for '%s': %w", cfg.Model, err)
			}

			log.Printf("[executor] ServiceManager provided endpoint: %s", modelEndpoint)
			cfg.Env = append(cfg.Env,
				"MODEL_URL="+modelEndpoint,
				"OPENAI_BASE_URL="+modelEndpoint+"/v1",
			)
		}
		if !hasAPIKey {
			cfg.Env = append(cfg.Env,
				"OPENAI_API_KEY=orpheus-internal-key",
			)
		}

		log.Printf("[executor] Agent uses model '%s'", cfg.Model)
	}

	// Generate entrypoint
	gen := generator.New()
	entrypointPath, err := gen.Generate(cfg, false) // sync execution
	if err != nil {
		return nil, fmt.Errorf("generate entrypoint: %w", err)
	}
	defer gen.Cleanup(entrypointPath)

	// Resolve image path
	imagePath := resolveImagePath(req.AgentPath)

	// Build execute options with streaming
	opts := &proxy.ExecuteOptions{
		Input:         marshalInput(req.Input),
		UseIsolate:    true,
		RootFSPath:    imagePath,
		WorkspacePath: resolveWorkspacePath(req.AgentPath),
		StreamWriter:  streamWriter, // Enable streaming
	}

	// Apply memory limits
	if req.Options.MemoryLimit > 0 {
		opts.MemoryLimit = req.Options.MemoryLimit
		opts.MemoryTarget = req.Options.MemoryLimit / 2
		opts.SwapEnabled = true
	}

	// Apply timeout options
	if req.Options.IdleTimeout > 0 {
		opts.IdleTimeout = time.Duration(req.Options.IdleTimeout) * time.Second
	}
	if req.Options.Timeout > 0 {
		opts.MaxTimeout = time.Duration(req.Options.Timeout) * time.Second
	}

	// Activity check interval defaults to 5 seconds
	opts.ActivityCheck = 5 * time.Second

	// Execute agent with streaming enabled
	result := proxy.RunAgent(ctx, cfg, entrypointPath, opts)
	return result, nil
}

// resolveWorkspacePath returns the workspace directory path for an agent.
// Workspace is stored at ~/.orpheus/workspaces/{agentName}/ or /var/lib/orpheus/workspaces/{agentName}/
func resolveWorkspacePath(agentPath string) string {
	// Get agent name from path
	agentName := filepath.Base(agentPath)
	if agentName == "agent" {
		agentName = filepath.Base(filepath.Dir(agentPath))
	}

	// Determine workspace base directory (mirrors agent base dir logic)
	workspaceBaseDir := "/var/lib/orpheus/workspaces"
	if _, err := os.Stat("/var/lib/orpheus"); os.IsNotExist(err) {
		home, _ := os.UserHomeDir()
		workspaceBaseDir = filepath.Join(home, ".orpheus", "workspaces")
	}

	// Try multiple possible home directories for Lima VM scenarios
	homeDirs := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		homeDirs = append(homeDirs, home)
	}

	// Extract home from agent path for Lima VM mounted macOS home
	if len(agentPath) > 7 && agentPath[:7] == "/Users/" {
		for i := 7; i < len(agentPath); i++ {
			if agentPath[i] == '/' {
				homeDirs = append(homeDirs, agentPath[:i])
				break
			}
		}
	}

	// Find existing workspace
	for _, home := range homeDirs {
		workspacePath := filepath.Join(home, ".orpheus", "workspaces", agentName)
		if _, err := os.Stat(workspacePath); err == nil {
			return workspacePath
		}
	}

	// Return default path (will be created by deploy_handler)
	return filepath.Join(workspaceBaseDir, agentName)
}

// resolveImagePath determines the rootfs path for the agent.
// It checks for deployed images in ~/.orpheus/agents/{name}/
// and falls back to the agent directory if not deployed.
//
// With base image merging, the directory structure is:
//   ~/.orpheus/agents/{name}/        <- rootfs (lib, usr, etc)
//   ~/.orpheus/agents/{name}/agent/  <- agent code (agent.yaml, etc)
//
// The agentPath may point to either the rootfs or the agent code directory.
func resolveImagePath(agentPath string) string {
	// Get agent name from path
	// If path ends with /agent, use parent directory name
	agentName := filepath.Base(agentPath)
	if agentName == "agent" {
		// Path is .../agents/{name}/agent, get the parent dir name
		agentName = filepath.Base(filepath.Dir(agentPath))
	}

	// Try multiple possible home directories:
	// 1. Current user's home (daemon might run as different user)
	// 2. Extract home from agent path (for Lima/Docker scenarios where macOS home is mounted)
	homeDirs := []string{}

	// Add current user home
	if home, err := os.UserHomeDir(); err == nil {
		homeDirs = append(homeDirs, home)
	}

	// Extract home directory from agent path if it looks like /Users/<name>/...
	// This handles Lima VM where macOS home is mounted at same path
	if len(agentPath) > 7 && agentPath[:7] == "/Users/" {
		// Find the home directory by locating the second slash after /Users/
		for i := 7; i < len(agentPath); i++ {
			if agentPath[i] == '/' {
				homeDirs = append(homeDirs, agentPath[:i])
				break
			}
		}
	}

	// Use deployed agent image
	// Try each home directory until we find a valid deployed agent
	for _, home := range homeDirs {
		deployedPath := filepath.Join(home, ".orpheus", "agents", agentName)

		// Verify it's a complete rootfs (must have /lib for dynamic linker)
		if _, err := os.Stat(filepath.Join(deployedPath, "lib")); err == nil {
			return deployedPath
		}
	}

	// No valid deployed image found - return original path as fallback
	// (will likely fail with clear error from runc)
	return agentPath
}

// marshalInput converts the input map to JSON string.
func marshalInput(input map[string]interface{}) string {
	if input == nil || len(input) == 0 {
		return "{}"
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(data)
}
