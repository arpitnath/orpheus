package mcp

import (
	"log"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPHandler handles MCP protocol requests via HTTP/SSE.
type MCPHandler struct {
	manager *MCPServerManager
	handler *sdk.StreamableHTTPHandler
}

// NewMCPHandler creates a new MCP HTTP handler.
func NewMCPHandler(manager *MCPServerManager) *MCPHandler {
	h := &MCPHandler{
		manager: manager,
	}

	// Create StreamableHTTPHandler from SDK
	// This handles all MCP protocol details (JSON-RPC, SSE, sessions)
	h.handler = sdk.NewStreamableHTTPHandler(
		func(r *http.Request) *sdk.Server {
			return h.routeToServer(r)
		},
		&sdk.StreamableHTTPOptions{
			SessionTimeout: 30 * time.Minute,
		},
	)

	return h
}

// routeToServer determines which MCP server to use for a request.
// This function is called by StreamableHTTPHandler for each new session.
//
// OSS Route format: /mcp/agents/{agent_name}
func (h *MCPHandler) routeToServer(r *http.Request) *sdk.Server {
	// Extract agent name from path: /mcp/agents/{agent_name}
	agentName := extractAgentName(r.URL.Path)
	if agentName == "" {
		log.Printf("MCP route parse error: no agent name in path: %s", r.URL.Path)
		return nil // SDK returns 400 Bad Request
	}

	// Get or create MCP server for this agent
	server, err := h.manager.GetOrCreateServer("", agentName)
	if err != nil {
		log.Printf("MCP server creation failed: %v (agent: %s)", err, agentName)
		return nil // SDK returns 400 Bad Request (agent not found)
	}

	log.Printf("MCP request routed to agent: %s", agentName)
	return server
}

// ServeHTTP implements http.Handler interface.
// Delegates to StreamableHTTPHandler which manages MCP protocol.
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// extractAgentName extracts the agent name from MCP path.
// Path format: /mcp/agents/{agent_name} or /mcp/agents/{agent_name}/...
func extractAgentName(path string) string {
	// Remove /mcp/ prefix
	path = strings.TrimPrefix(path, "/mcp/")

	// Expected: agents/{agent_name}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "agents" {
		return ""
	}

	return parts[1]
}
