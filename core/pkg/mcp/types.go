package mcp

import "time"

// MCPConfig configures MCP server behavior.
type MCPConfig struct {
	Enabled         bool          // Enable MCP endpoints
	BasePath        string        // Base URL path (default: "/mcp")
	SessionTimeout  time.Duration // Session idle timeout
	MaxSessions     int           // Maximum concurrent sessions
	ProgressUpdates bool          // Enable progress streaming
}

// DefaultMCPConfig returns default MCP configuration.
func DefaultMCPConfig() *MCPConfig {
	return &MCPConfig{
		Enabled:         true,
		BasePath:        "/mcp",
		SessionTimeout:  30 * time.Minute,
		MaxSessions:     1000,
		ProgressUpdates: true,
	}
}
