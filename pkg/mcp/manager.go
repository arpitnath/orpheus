package mcp

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerGetter provides access to AgentScale server for MCP integration.
// This interface allows MCP package to access server without circular dependency.
type ServerGetter interface {
	GetAgentInstance(agentID string) (AgentInstance, error)
}

// AgentInstance represents the minimal interface needed from server.AgentInstance.
type AgentInstance interface {
	GetQueue() RequestQueue
	GetConfig() AgentConfig
}

// RequestQueue represents the minimal queue interface needed.
type RequestQueue interface {
	Enqueue(req Request) error
}

// AgentConfig represents the minimal config interface.
type AgentConfig interface {
	GetName() string
	GetDescription() string
}

// Request represents a minimal request interface.
type Request interface {
	GetID() string
	GetInput() []byte
	GetResponseChannel() chan Response
}

// Response represents a minimal response interface.
type Response interface {
	GetResult() Result
	GetError() error
	GetDuration() int64
}

// Result represents execution result.
type Result interface {
	GetStatus() string
	GetOutput() map[string]interface{}
	GetError() string
}

// MCPServerManager manages MCP servers per agent with multi-tenant isolation.
type MCPServerManager struct {
	servers      map[string]*sdk.Server // key: "orgID:agentName"
	serverGetter ServerGetter           // Access to AgentScale server
	mu           sync.RWMutex
	logger       *slog.Logger
}

// NewMCPServerManager creates a new MCP server manager.
func NewMCPServerManager(serverGetter ServerGetter) *MCPServerManager {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	return &MCPServerManager{
		servers:      make(map[string]*sdk.Server),
		serverGetter: serverGetter,
		logger:       logger,
	}
}

// GetOrCreateServer returns an MCP server for the specified agent.
// Creates a new server if one doesn't exist. Thread-safe.
func (m *MCPServerManager) GetOrCreateServer(orgID, agentName string) (*sdk.Server, error) {
	key := fmt.Sprintf("%s:%s", orgID, agentName)

	// Try read lock first (fast path for existing servers)
	m.mu.RLock()
	if server, ok := m.servers[key]; ok {
		m.mu.RUnlock()
		return server, nil
	}
	m.mu.RUnlock()

	// Need to create - acquire write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if server, ok := m.servers[key]; ok {
		return server, nil
	}

	// Get AgentInstance from base server
	instance, err := m.serverGetter.GetAgentInstance(agentName)
	if err != nil {
		return nil, fmt.Errorf("get agent instance: %w", err)
	}

	// Create new MCP server for this agent
	server := sdk.NewServer(
		&sdk.Implementation{
			Name:    fmt.Sprintf("agentscale-%s", agentName),
			Version: "1.0.0",
		},
		&sdk.ServerOptions{
			Capabilities: &sdk.ServerCapabilities{
				Tools: &sdk.ToolCapabilities{},
			},
		},
	)

	// Register tools for this agent
	if err := registerAgentTools(server, instance, orgID, agentName); err != nil {
		return nil, fmt.Errorf("register tools: %w", err)
	}

	// Cache the server
	m.servers[key] = server

	m.logger.Info("created MCP server",
		"org_id", orgID,
		"agent", agentName,
		"key", key)

	return server, nil
}

// RemoveServer removes an MCP server from the cache.
// Called when an agent is undeployed.
func (m *MCPServerManager) RemoveServer(orgID, agentName string) {
	key := fmt.Sprintf("%s:%s", orgID, agentName)

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.servers, key)

	m.logger.Info("removed MCP server",
		"org_id", orgID,
		"agent", agentName)
}

// GetServerCount returns the number of cached MCP servers.
func (m *MCPServerManager) GetServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.servers)
}
