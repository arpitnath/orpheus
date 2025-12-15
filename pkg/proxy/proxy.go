// Package proxy handles agent execution and output capture.
package proxy

import (
	"context"
	"time"

	"agentscale/pkg/config"
)

// Proxy wraps agent execution with configuration
type Proxy struct {
	cfg *config.AgentConfig
}

// New creates a new Proxy for the given agent configuration
func New(cfg *config.AgentConfig) *Proxy {
	return &Proxy{cfg: cfg}
}

// Execute runs the agent with the given entry point and options
func (p *Proxy) Execute(ctx context.Context, entrypointPath string, opts *ExecuteOptions) *Result {
	// Create context with timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && p.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
	}

	return RunAgent(ctx, p.cfg, entrypointPath, opts)
}

// ExecuteWithTimeout runs the agent with a specific timeout
func (p *Proxy) ExecuteWithTimeout(entrypointPath string, timeout time.Duration, opts *ExecuteOptions) *Result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return RunAgent(ctx, p.cfg, entrypointPath, opts)
}

// Config returns the proxy's configuration
func (p *Proxy) Config() *config.AgentConfig {
	return p.cfg
}
