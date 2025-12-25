package mcp

import (
	"fmt"
	"net/http"
	"strings"
)

// ParseMCPRoute parses MCP URL path and extracts org_id and agent_name.
// Expected format: /mcp/{org_id}/agents/{agent_name}
func ParseMCPRoute(r *http.Request) (*MCPRoute, error) {
	// Split path by '/'
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	// Validate structure: mcp / org_id / agents / agent_name
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid MCP path format (expected: /mcp/{org_id}/agents/{agent_name})")
	}

	if parts[0] != "mcp" {
		return nil, fmt.Errorf("path must start with /mcp/")
	}

	if parts[2] != "agents" {
		return nil, fmt.Errorf("invalid namespace (expected 'agents', got '%s')", parts[2])
	}

	orgID := parts[1]
	agentName := parts[3]

	// Validate components are non-empty
	if orgID == "" {
		return nil, fmt.Errorf("org_id cannot be empty")
	}

	if agentName == "" {
		return nil, fmt.Errorf("agent_name cannot be empty")
	}

	return &MCPRoute{
		OrgID:     orgID,
		AgentName: agentName,
	}, nil
}

// ValidateRoute performs additional validation on a parsed route.
func ValidateRoute(route *MCPRoute) error {
	if route == nil {
		return fmt.Errorf("route is nil")
	}

	// Validate org_id format (alphanumeric + hyphens only)
	if !isValidIdentifier(route.OrgID) {
		return fmt.Errorf("invalid org_id format: %s", route.OrgID)
	}

	// Validate agent_name format
	if !isValidIdentifier(route.AgentName) {
		return fmt.Errorf("invalid agent_name format: %s", route.AgentName)
	}

	return nil
}

// isValidIdentifier checks if string is a valid identifier.
// Allows: alphanumeric, hyphens, underscores
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}

	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_') {
			return false
		}
	}

	return true
}
