// Package daemon provides MCP adapters for daemon mode execution.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"orpheus/daemon/pkg/config"
	"orpheus/daemon/pkg/execlog"
	"orpheus/daemon/pkg/mcp"
	"orpheus/daemon/pkg/proxy"

	"github.com/google/uuid"
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

	go func() {
		ctx := context.Background()
		requestID := uuid.New().String()

		// Helper to log execlog events with MCP source
		logExecEvent := func(state string, durationMs *int64, errMsg *string) {

			agentName := filepath.Base(filepath.Dir(q.agentPath))
			writer, err := execlog.NewWriter(q.server.execlogDir, agentName)
			if err != nil {
				log.Printf("[mcp-adapter] execlog writer error: %v", err)
				return
			}

			event := &execlog.Event{
				Timestamp:  time.Now(),
				RequestID:  requestID,
				State:      state,
				DurationMs: durationMs,
				Error:      errMsg,
				Source:     ptrString(execlog.SourceMCP),
			}

			if err := writer.Log(event); err != nil {
				log.Printf("[mcp-adapter] execlog write error: %v", err)
			}
		}

		logExecEvent(execlog.StateQueued, nil, nil)

		var inputMap map[string]interface{}
		if err := json.Unmarshal(req.GetInput(), &inputMap); err != nil {

			errStr := fmt.Sprintf("parse input: %v", err)
			logExecEvent(execlog.StateFailed, nil, &errStr)

			errResp := &daemonMCPResponse{
				err: fmt.Errorf("parse input: %w", err),
			}
			req.GetResponseChannel() <- errResp
			return
		}

		logExecEvent(execlog.StateStarted, nil, nil)
		startTime := time.Now()

		runReq := &RunRequest{
			AgentPath: q.agentPath,
			Input:     inputMap,
		}

		result, err := Execute(ctx, runReq, q.server.serviceManager)

		// Calculate duration
		durationMs := time.Since(startTime).Milliseconds()

		if err != nil || result.Status != proxy.StatusSuccess {
			var errStr string
			if err != nil {
				errStr = err.Error()
			} else if result.Error != "" {
				errStr = result.Error
			}
			logExecEvent(execlog.StateFailed, &durationMs, &errStr)
		} else {
			logExecEvent(execlog.StateCompleted, &durationMs, nil)
		}

		resp := &daemonMCPResponse{
			result: result,
			err:    err,
		}

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
