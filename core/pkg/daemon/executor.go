package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"orpheus/daemon/pkg/config"
	"orpheus/daemon/pkg/generator"
	"orpheus/daemon/pkg/proxy"
	"orpheus/daemon/pkg/runtime"
)

// Execute runs an agent using the existing proxy.RunAgent infrastructure.
// This is the bridge between the daemon API and the execution engine.
func Execute(ctx context.Context, req *RunRequest) (*proxy.Result, error) {
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
		Input:      marshalInput(req.Input),
		UseIsolate: true,
		RootFSPath: imagePath,
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
func ExecuteStreaming(ctx context.Context, req *RunRequest, streamWriter runtime.StreamWriter) (*proxy.Result, error) {
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
		Input:        marshalInput(req.Input),
		UseIsolate:   true,
		RootFSPath:   imagePath,
		StreamWriter: streamWriter, // Enable streaming
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
