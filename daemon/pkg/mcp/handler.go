package mcp

import (
	"log"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orpheus/daemon/pkg/auth"
)

// MCPHandler handles MCP protocol requests via HTTP/SSE.
type MCPHandler struct {
	manager     *MCPServerManager
	authStore   *auth.Store
	rateLimiter *auth.RateLimiter
	handler     *sdk.StreamableHTTPHandler
}

// NewMCPHandler creates a new MCP HTTP handler.
func NewMCPHandler(
	manager *MCPServerManager,
	authStore *auth.Store,
	rateLimiter *auth.RateLimiter,
) *MCPHandler {
	h := &MCPHandler{
		manager:     manager,
		authStore:   authStore,
		rateLimiter: rateLimiter,
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
func (h *MCPHandler) routeToServer(r *http.Request) *sdk.Server {
	// 1. Parse MCP route from URL
	route, err := ParseMCPRoute(r)
	if err != nil {
		log.Printf("MCP route parse error: %v (path: %s)", err, r.URL.Path)
		return nil // SDK returns 400 Bad Request
	}

	// 2. Extract and validate API key
	apiKey := extractBearerToken(r.Header.Get("Authorization"))
	if apiKey == "" {
		log.Printf("MCP request missing API key")
		return nil // SDK returns 400 Bad Request
	}

	// 3. Validate API key and get organization
	keyData, err := h.authStore.ValidateKey(apiKey)
	if err != nil {
		log.Printf("MCP auth failed: %v", err)
		return nil // SDK returns 400 Bad Request
	}

	// 4. Verify org_id in URL matches API key's organization
	if keyData.OrgID != route.OrgID {
		log.Printf("MCP org mismatch: URL has %s, key has %s", route.OrgID, keyData.OrgID)
		return nil // SDK returns 403 Forbidden (via 400)
	}

	// 5. Check rate limit
	if !h.rateLimiter.Allow(apiKey, keyData.RequestsPerMinute) {
		log.Printf("MCP rate limit exceeded for key: %s", keyData.Name)
		return nil // SDK returns 400 Bad Request
	}

	// 6. Get or create MCP server for this agent
	server, err := h.manager.GetOrCreateServer(route.OrgID, route.AgentName)
	if err != nil {
		log.Printf("MCP server creation failed: %v (org: %s, agent: %s)",
			err, route.OrgID, route.AgentName)
		return nil // SDK returns 400 Bad Request (agent not found)
	}

	log.Printf("MCP request routed to agent: %s (org: %s)", route.AgentName, route.OrgID)
	return server
}

// ServeHTTP implements http.Handler interface.
// Delegates to StreamableHTTPHandler which manages MCP protocol.
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// extractBearerToken extracts the token from "Bearer <token>" format.
func extractBearerToken(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
