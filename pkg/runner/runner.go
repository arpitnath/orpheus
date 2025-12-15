// Package runner orchestrates the full agent execution lifecycle.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"agentscale/pkg/config"
	"agentscale/pkg/generator"
	"agentscale/pkg/proxy"
)

// Runner orchestrates agent loading, generation, and execution
type Runner struct {
	cfg       *config.AgentConfig
	generator *generator.Generator
	proxy     *proxy.Proxy
}

// RunOptions holds options for running an agent
type RunOptions struct {
	// Input is the JSON input string (if empty, reads from stdin)
	Input string

	// NoIsolate skips container isolation
	NoIsolate bool

	// KeepEntrypoint preserves the generated _entrypoint.py
	KeepEntrypoint bool

	// Env contains additional environment variables
	Env []string

	// Stdin is an optional reader for input (defaults to os.Stdin)
	Stdin io.Reader
}

// New creates a new Runner for the given agent configuration
func New(cfg *config.AgentConfig) *Runner {
	return &Runner{
		cfg:       cfg,
		generator: generator.New(),
		proxy:     proxy.New(cfg),
	}
}

// Run executes the agent and returns the result
func (r *Runner) Run(ctx context.Context, opts *RunOptions) (*proxy.Result, error) {
	if opts == nil {
		opts = &RunOptions{}
	}

	// Generate entry point (unified template handles both sync and async)
	entrypointPath, err := r.generator.Generate(r.cfg, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate entrypoint: %w", err)
	}

	// Cleanup entrypoint on exit (unless KeepEntrypoint is set)
	if !opts.KeepEntrypoint {
		defer r.generator.Cleanup(entrypointPath)
	}

	// Get input
	input := opts.Input
	if input == "" {
		// Read from stdin
		stdin := opts.Stdin
		if stdin == nil {
			stdin = os.Stdin
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
		input = string(data)
	}

	// Determine isolation settings
	useIsolate := !opts.NoIsolate
	isolatePath := ""
	if useIsolate {
		isolatePath = findIsolateBinary()
		if isolatePath == "" {
			// Fall back to no isolation if binary not found
			useIsolate = false
		}
	}

	// Execute agent
	execOpts := &proxy.ExecuteOptions{
		Input:       input,
		Env:         opts.Env,
		UseIsolate:  useIsolate,
		IsolatePath: isolatePath,
	}

	result := r.proxy.Execute(ctx, entrypointPath, execOpts)
	return result, nil
}

// RunFromDir loads config from directory and executes the agent
func RunFromDir(ctx context.Context, agentDir string, opts *RunOptions) (*proxy.Result, error) {
	// Load configuration
	cfg, err := config.Load(agentDir)
	if err != nil {
		return nil, err
	}

	// Create runner and execute
	runner := New(cfg)
	return runner.Run(ctx, opts)
}

// Config returns the runner's configuration
func (r *Runner) Config() *config.AgentConfig {
	return r.cfg
}

// findIsolateBinary searches for the isolate binary in known locations
func findIsolateBinary() string {
	// Search order:
	// 1. ./isolate/bin/isolate (embedded in agentscale - preferred)
	// 2. ./isolation/bin/isolate (relative to cwd - development)
	// 3. ../isolation/bin/isolate (sibling directory - monorepo development)
	// 4. ~/.agentscale/bin/isolate (user installation)
	// 5. /usr/local/bin/isolate (system installation)

	candidates := []string{
		"./isolate/bin/isolate",
		"./isolation/bin/isolate",
		"../isolation/bin/isolate",
	}

	// Add user home directory path
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".agentscale", "bin", "isolate"))
	}

	// Add system path
	candidates = append(candidates, "/usr/local/bin/isolate")

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			// Convert to absolute path
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
			return path
		}
	}

	return ""
}

// OutputJSON formats a result as JSON string
func OutputJSON(result *proxy.Result) string {
	// Create output structure
	output := map[string]interface{}{
		"status":    result.Status,
		"exit_code": result.ExitCode,
		"duration":  result.Duration.String(),
	}

	if result.Output != nil {
		output["output"] = result.Output
	}
	if result.Error != "" {
		output["error"] = result.Error
	}
	if result.Stderr != "" {
		output["stderr"] = result.Stderr
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal result: %s"}`, err)
	}
	return string(data)
}
