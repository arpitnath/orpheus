// Package registry manages deployed agent metadata for discovery and execution.
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RegisteredAgent represents a deployed agent's metadata.
type RegisteredAgent struct {
	Name        string    `json:"name"`         // Agent name (unique identifier)
	Runtime     string    `json:"runtime"`      // Runtime type (python3, nodejs20)
	Path        string    `json:"path"`         // Server-side agent directory path
	ResolvedEnv []string  `json:"resolved_env"` // Pre-resolved environment variables
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Registry manages agent registration and discovery.
type Registry interface {
	Register(agent RegisteredAgent) error
	Get(name string) (*RegisteredAgent, error)
	List() ([]RegisteredAgent, error)
	Delete(name string) error
}

// FileRegistry implements Registry using file-based storage.
type FileRegistry struct {
	basePath string
}

// NewFileRegistry creates a new file-based agent registry.
func NewFileRegistry(basePath string) (*FileRegistry, error) {
	// Ensure registry directory exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("create registry directory: %w", err)
	}

	return &FileRegistry{
		basePath: basePath,
	}, nil
}

// Register adds or updates an agent in the registry.
func (r *FileRegistry) Register(agent RegisteredAgent) error {
	// Set timestamps
	if agent.CreatedAt.IsZero() {
		agent.CreatedAt = time.Now()
	}
	agent.UpdatedAt = time.Now()

	// Write to file
	path := filepath.Join(r.basePath, agent.Name+".json")
	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write registry file: %w", err)
	}

	return nil
}

// Get retrieves an agent by name from the registry.
func (r *FileRegistry) Get(name string) (*RegisteredAgent, error) {
	path := filepath.Join(r.basePath, name+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("agent not found: %s", name)
		}
		return nil, fmt.Errorf("read registry file: %w", err)
	}

	var agent RegisteredAgent
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("unmarshal agent: %w", err)
	}

	return &agent, nil
}

// List returns all registered agents.
func (r *FileRegistry) List() ([]RegisteredAgent, error) {
	entries, err := os.ReadDir(r.basePath)
	if err != nil {
		return nil, fmt.Errorf("read registry directory: %w", err)
	}

	var agents []RegisteredAgent
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		name := entry.Name()[:len(entry.Name())-5] // Remove .json
		agent, err := r.Get(name)
		if err != nil {
			continue // Skip invalid entries
		}

		agents = append(agents, *agent)
	}

	return agents, nil
}

// Delete removes an agent from the registry.
func (r *FileRegistry) Delete(name string) error {
	path := filepath.Join(r.basePath, name+".json")

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("agent not found: %s", name)
		}
		return fmt.Errorf("delete registry file: %w", err)
	}

	return nil
}
