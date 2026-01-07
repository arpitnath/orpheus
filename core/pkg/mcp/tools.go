package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ExecuteInput is the input for the agent execution tool.
type ExecuteInput struct {
	Input map[string]interface{} `json:"input" jsonschema:"Input data to pass to the agent"`
}

// ExecuteOutput is the output from agent execution.
type ExecuteOutput struct {
	Status   string                 `json:"status" jsonschema:"Execution status (success, error, timeout)"`
	Output   map[string]interface{} `json:"output,omitempty" jsonschema:"Agent output data"`
	Error    string                 `json:"error,omitempty" jsonschema:"Error message if failed"`
	Duration int64                  `json:"duration_ms" jsonschema:"Execution duration in milliseconds"`
}

// registerAgentTools registers MCP tools for an agent.
// For v0.1.0, registers a single "execute" tool that runs the agent.
func registerAgentTools(
	mcpServer *sdk.Server,
	instance AgentInstance,
	orgID, agentName string,
) error {
	// Primary tool: Execute the agent
	executeTool := &sdk.Tool{
		Name:        "execute",
		Description: fmt.Sprintf("Execute the %s agent with given input", agentName),
	}

	// Create tool handler
	handler := createExecuteToolHandler(instance, orgID, agentName)

	// Register with MCP server using SDK's AddTool
	sdk.AddTool(mcpServer, executeTool, handler)

	return nil
}

// createExecuteToolHandler creates the MCP tool handler for agent execution.
// This handler converts MCP requests to AgentScale's scaling.Request format,
// enqueues to the agent's queue, and waits for the worker pool to execute.
func createExecuteToolHandler(
	instance AgentInstance,
	orgID, agentName string,
) func(context.Context, *sdk.CallToolRequest, ExecuteInput) (*sdk.CallToolResult, ExecuteOutput, error) {
	return func(
		ctx context.Context,
		req *sdk.CallToolRequest,
		input ExecuteInput,
	) (*sdk.CallToolResult, ExecuteOutput, error) {
		// 1. Convert input to JSON bytes
		inputJSON, err := json.Marshal(input.Input)
		if err != nil {
			return nil, ExecuteOutput{}, fmt.Errorf("marshal input: %w", err)
		}

		// 2. Create request object (implements mcp.Request interface)
		request := &mcpRequestImpl{
			id:         generateRequestID(),
			input:      inputJSON,
			ctx:        ctx,
			responseCh: make(chan Response, 1),
		}

		// 3. Enqueue via interface (queue adapter handles conversion)
		queue := instance.GetQueue()
		if err := queue.Enqueue(request); err != nil {
			return nil, ExecuteOutput{}, fmt.Errorf("enqueue: %w", err)
		}

		// 4. Wait for response from worker pool
		select {
		case resp := <-request.responseCh:
			// Check for execution error
			if resp.GetError() != nil {
				return nil, ExecuteOutput{}, resp.GetError()
			}

			// Convert to MCP output format
			result := resp.GetResult()
			output := ExecuteOutput{
				Status:   result.GetStatus(),
				Output:   result.GetOutput(),
				Error:    result.GetError(),
				Duration: resp.GetDuration(),
			}

			return nil, output, nil

		case <-ctx.Done():
			return nil, ExecuteOutput{}, ctx.Err()
		}
	}
}

// mcpRequestImpl implements the mcp.Request interface.
type mcpRequestImpl struct {
	id         string
	input      []byte
	ctx        context.Context
	responseCh chan Response
}

func (r *mcpRequestImpl) GetID() string                    { return r.id }
func (r *mcpRequestImpl) GetInput() []byte                 { return r.input }
func (r *mcpRequestImpl) GetResponseChannel() chan Response { return r.responseCh }

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	// Use nanosecond timestamp for unique ID
	return fmt.Sprintf("mcp-req-%d", time.Now().UnixNano())
}
