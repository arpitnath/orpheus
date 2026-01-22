package mcp

import "errors"

// Common MCP-related errors
var (
	// ErrAgentNotFound indicates the requested agent doesn't exist
	ErrAgentNotFound = errors.New("agent not found")

	// ErrInvalidRoute indicates the MCP URL path is malformed
	ErrInvalidRoute = errors.New("invalid MCP route")

	// ErrQueueFull indicates the agent's queue is at capacity
	ErrQueueFull = errors.New("agent queue is full")

	// ErrSessionNotFound indicates MCP session doesn't exist
	ErrSessionNotFound = errors.New("session not found")
)
