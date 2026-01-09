package service

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
)

// Manager manages model servers (platform-aware)
type Manager struct {
	servers map[string]ModelServer
	mu      sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new service manager
func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]ModelServer),
	}
}

// Start initializes the service manager
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Start health check goroutine
	m.wg.Add(1)
	go m.healthCheckLoop()

	log.Printf("[service-manager] Started")
	return nil
}

// EnsureModelServer ensures a model server is running for the given config
func (m *Manager) EnsureModelServer(ctx context.Context, config ModelConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if server already exists for this model
	serverKey := fmt.Sprintf("%s-%s", config.Server, config.Name)
	if server, exists := m.servers[serverKey]; exists {
		// Verify it's healthy
		if healthy, _ := server.Health(ctx); healthy {
			log.Printf("[service-manager] Reusing existing server for %s", config.Name)
			return server.Endpoint(), nil
		}

		// Unhealthy, restart it
		log.Printf("[service-manager] Existing server unhealthy, restarting...")
		if err := server.Restart(ctx); err != nil {
			return "", fmt.Errorf("restart server: %w", err)
		}
		return server.Endpoint(), nil
	}

	// Create new server based on platform
	server, err := m.createServer(config)
	if err != nil {
		return "", fmt.Errorf("create server: %w", err)
	}

	// Start the server
	if err := server.Start(ctx); err != nil {
		return "", fmt.Errorf("start server: %w", err)
	}

	// Cache for reuse
	m.servers[serverKey] = server

	log.Printf("[service-manager] Model server ready: %s at %s", config.Name, server.Endpoint())
	return server.Endpoint(), nil
}

// createServer creates the appropriate server based on platform
func (m *Manager) createServer(config ModelConfig) (ModelServer, error) {
	// Auto-detect if not specified
	serverType := config.Server
	if serverType == "" || serverType == "auto" {
		serverType = m.detectServerType()
	}

	switch serverType {
	case "ollama":
		return NewOllamaServer(config.Name), nil

	// TODO: Add vLLM for Linux
	// case "vllm":
	//     return NewVLLMServer(config.Name), nil

	default:
		return nil, fmt.Errorf("unsupported server type: %s", serverType)
	}
}

// detectServerType auto-detects the best server for current platform
func (m *Manager) detectServerType() string {
	// For now, always use Ollama
	// TODO: Detect Linux + GPU -> use vLLM
	if runtime.GOOS == "darwin" {
		return "ollama"
	}

	// Linux default (for now)
	return "ollama"
}

// healthCheckLoop monitors all servers
func (m *Manager) healthCheckLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[service-manager] Health check loop stopped")
			return

		case <-ticker.C:
			m.checkAllServers()
		}
	}
}

// checkAllServers checks health of all managed servers
func (m *Manager) checkAllServers() {
	m.mu.RLock()
	servers := make(map[string]ModelServer)
	for k, v := range m.servers {
		servers[k] = v
	}
	m.mu.RUnlock()

	for key, server := range servers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		healthy, err := server.Health(ctx)
		cancel()

		if err != nil || !healthy {
			log.Printf("[service-manager] Server %s unhealthy, restarting...", key)

			// Try to restart
			restartCtx, restartCancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := server.Restart(restartCtx); err != nil {
				log.Printf("[service-manager] Failed to restart %s: %v", key, err)
			}
			restartCancel()
		}
	}
}

// Stop stops all managed servers
func (m *Manager) Stop(ctx context.Context) error {
	log.Printf("[service-manager] Stopping all servers...")

	// Stop health check loop
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()

	// Stop all servers
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, server := range m.servers {
		log.Printf("[service-manager] Stopping %s...", key)
		if err := server.Stop(ctx); err != nil {
			log.Printf("[service-manager] Error stopping %s: %v", key, err)
		}
	}

	log.Printf("[service-manager] All servers stopped")
	return nil
}
