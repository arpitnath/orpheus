// Package daemon provides MCP adapters for daemon mode execution.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"

	"orpheus/daemon/pkg/config"
	"orpheus/daemon/pkg/mcp"
	"orpheus/daemon/pkg/proxy"
)

// daemonServerGetter implements mcp.ServerGetter for daemon mode.
// It uses the registry to look up deployed agents (consistent with HTTP handler).
type daemonServerGetter struct {
	server *Server
}

// NewDaemonServerGetter creates a new ServerGetter for daemon mode.
func NewDaemonServerGetter(server *Server) mcp.ServerGetter {
	return &daemonServerGetter{server: server}
}

// GetAgentInstance returns an agent instance for MCP execution.
func (d *daemonServerGetter) GetAgentInstance(agentID string) (mcp.AgentInstance, error) {
	// Use registry to get the correct agent path (consistent with HTTP handler)
	agent, err := d.server.registry.Get(agentID)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	agentPath := agent.Path

	// Load agent config to verify it's valid
	cfg, err := config.Load(agentPath)
	if err != nil {
		return nil, fmt.Errorf("load agent config: %w", err)
	}

	return &daemonAgentInstance{
		server:    d.server,
		agentPath: agentPath,
		config:    cfg,
	}, nil
}

// daemonAgentInstance implements mcp.AgentInstance for daemon mode.
type daemonAgentInstance struct {
	server    *Server
	agentPath string
	config    *config.AgentConfig
}

// GetQueue returns a queue adapter that executes directly.
func (d *daemonAgentInstance) GetQueue() mcp.RequestQueue {
	return &daemonDirectQueue{
		server:    d.server,
		agentPath: d.agentPath,
	}
}

// GetConfig returns the agent configuration.
func (d *daemonAgentInstance) GetConfig() mcp.AgentConfig {
	return &daemonConfigAdapter{cfg: d.config}
}

// daemonDirectQueue implements mcp.RequestQueue by calling Execute() directly.
// Unlike pkg/server which uses queues and worker pools, daemon executes synchronously.
type daemonDirectQueue struct {
	server    *Server
	agentPath string
}

// Enqueue executes the agent and sends response to the request's channel.
func (q *daemonDirectQueue) Enqueue(req mcp.Request) error {
	// Execute in a goroutine to not block the MCP handler
	go func() {
		ctx := context.Background()

		// Parse input from request
		var inputMap map[string]interface{}
		if err := json.Unmarshal(req.GetInput(), &inputMap); err != nil {
			// Send error response
			errResp := &daemonMCPResponse{
				err: fmt.Errorf("parse input: %w", err),
			}
			req.GetResponseChannel() <- errResp
			return
		}

		// Build RunRequest for daemon executor
		runReq := &RunRequest{
			AgentPath: q.agentPath,
			Input:     inputMap,
		}

		// Execute using daemon's executor (pass ServiceManager for model server management)
		result, err := Execute(ctx, runReq, q.server.serviceManager)

		// Build MCP response
		resp := &daemonMCPResponse{
			result: result,
			err:    err,
		}

		// Send to MCP request's response channel
		req.GetResponseChannel() <- resp
	}()

	return nil
}

// daemonMCPResponse implements mcp.Response interface.
type daemonMCPResponse struct {
	result *proxy.Result
	err    error
}

// GetResult returns the execution result.
func (r *daemonMCPResponse) GetResult() mcp.Result {
	if r.result == nil {
		return &daemonMCPResult{}
	}
	return &daemonMCPResult{result: r.result}
}

// GetError returns any execution error.
func (r *daemonMCPResponse) GetError() error {
	return r.err
}

// GetDuration returns execution duration in milliseconds.
func (r *daemonMCPResponse) GetDuration() int64 {
	if r.result == nil {
		return 0
	}
	return r.result.Duration.Milliseconds()
}

// daemonMCPResult implements mcp.Result interface.
type daemonMCPResult struct {
	result *proxy.Result
}

// GetStatus returns the execution status.
func (r *daemonMCPResult) GetStatus() string {
	if r.result == nil {
		return "unknown"
	}
	return string(r.result.Status)
}

// GetOutput returns the agent output.
func (r *daemonMCPResult) GetOutput() map[string]interface{} {
	if r.result == nil {
		return nil
	}
	return r.result.Output
}

// GetError returns any error message from the agent.
func (r *daemonMCPResult) GetError() string {
	if r.result == nil {
		return ""
	}
	return r.result.Error
}

// daemonConfigAdapter implements mcp.AgentConfig interface.
type daemonConfigAdapter struct {
	cfg *config.AgentConfig
}

// GetName returns the agent name.
func (a *daemonConfigAdapter) GetName() string {
	if a.cfg == nil {
		return ""
	}
	return a.cfg.Name
}

// GetDescription returns a description of the agent.
func (a *daemonConfigAdapter) GetDescription() string {
	if a.cfg == nil {
		return ""
	}
	// AgentConfig doesn't have a description field, return a formatted name
	return fmt.Sprintf("AgentScale agent: %s", a.cfg.Name)
}
