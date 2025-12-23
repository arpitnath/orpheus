package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"agentscale/pkg/config"
	"agentscale/pkg/generator"
	"agentscale/pkg/proxy"
)

// Execute runs an agent using the existing proxy.RunAgent infrastructure.
// This is the bridge between the daemon API and the execution engine.
func Execute(ctx context.Context, req *RunRequest) (*proxy.Result, error) {
	// Load agent config
	cfg, err := config.Load(req.AgentPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
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

// resolveImagePath determines the rootfs path for the agent.
// In the full implementation, this would look up the deployed image.
// For now, we use a convention-based approach.
func resolveImagePath(agentPath string) string {
	// Check if there's a deployed image in ~/.agentscale/agents/
	// If not, fall back to the agent directory itself (for development)

	// Get agent name from path
	agentName := filepath.Base(agentPath)

	// Check for deployed image
	// TODO: This should use a proper image registry
	// For now, return empty to let the executor handle it
	_ = agentName

	// Return the agent path as the rootfs for now
	// The deploy command creates a proper rootfs structure
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
