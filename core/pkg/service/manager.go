package service

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
)

// Manager manages model servers using actor model (race-free)
type Manager struct {
	commands      chan Command
	done          chan struct{}
	subscriptions []StateSubscription

	servers map[string]ModelServer
	state   ServerState
	backend ModelServer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	pidFilePath string
}

// NewManager creates a new service manager
func NewManager() *Manager {
	return &Manager{
		servers:     make(map[string]ModelServer),
		commands:    make(chan Command, 10),
		done:        make(chan struct{}),
		state:       StateStopped,
		pidFilePath: "/tmp/orpheus-model-server.lock",
	}
}

// Start initializes the service manager
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Start actor control loop
	m.wg.Add(1)
	go m.controlLoop()

	log.Printf("[service-manager] Started (actor model)")
	return nil
}

func (m *Manager) controlLoop() {
	defer m.wg.Done()

	for {
		select {
		case cmd := <-m.commands:
			m.handleCommand(cmd)

		case <-m.done:
			m.cleanup()
			return
		}
	}
}

func (m *Manager) handleCommand(cmd Command) {
	var err error

	switch cmd.Action {
	case CmdEnsureRunning:
		err = m.ensureRunning(cmd.Ctx)

	case CmdStop:
		err = m.stop(cmd.Ctx)

	case CmdSubscribe:
		m.subscriptions = append(m.subscriptions, StateSubscription{
			TargetState: cmd.TargetState,
			NotifyChan:  cmd.NotifyChan,
		})
		return
	}

	if cmd.Response != nil {
		cmd.Response <- err
	}
}

func (m *Manager) cleanup() {
	log.Printf("[service-manager] Shutting down")

	for _, server := range m.servers {
		server.Stop(context.Background())
	}
}

func (m *Manager) ensureRunning(ctx context.Context) error {
	if m.state == StateReady {
		return nil
	}

	if m.backend == nil {
		return fmt.Errorf("no backend configured")
	}

	unlock, err := acquirePIDLock(m.pidFilePath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer unlock()

	if healthy, _ := m.backend.Health(ctx); healthy {
		m.state = StateReady
		m.notifySubscribers()
		return nil
	}

	m.state = StateStarting
	m.notifySubscribers()

	if err := m.backend.Start(ctx); err != nil {
		m.state = StateStopped
		m.notifySubscribers()
		return err
	}

	m.state = StateReady
	m.notifySubscribers()

	return nil
}

func (m *Manager) stop(ctx context.Context) error {
	if m.backend == nil {
		return nil
	}

	m.state = StateStopped
	m.notifySubscribers()

	return m.backend.Stop(ctx)
}

func (m *Manager) notifySubscribers() {
	remaining := []StateSubscription{}

	for _, sub := range m.subscriptions {
		if sub.TargetState == m.state {
			select {
			case sub.NotifyChan <- nil:
			default:
			}
		} else {
			remaining = append(remaining, sub)
		}
	}

	m.subscriptions = remaining
}

func (m *Manager) waitForState(targetState ServerState, ctx context.Context) error {
	notifyChan := make(chan error, 1)

	m.commands <- Command{
		Action:      CmdSubscribe,
		TargetState: targetState,
		NotifyChan:  notifyChan,
	}

	select {
	case err := <-notifyChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// EnsureModelServer ensures a model server is running for the given config
func (m *Manager) EnsureModelServer(ctx context.Context, config ModelConfig) (string, error) {
	resp := make(chan error, 1)

	m.commands <- Command{
		Action:   CmdEnsureRunning,
		Ctx:      ctx,
		Response: resp,
	}

	select {
	case err := <-resp:
		if err != nil {
			return "", err
		}

		if m.backend != nil {
			return m.backend.Endpoint(), nil
		}

		return "", fmt.Errorf("backend not initialized")

	case <-ctx.Done():
		return "", ctx.Err()
	}
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

// Stop stops all managed servers
func (m *Manager) Stop(ctx context.Context) error {
	close(m.done)
	m.wg.Wait()

	log.Printf("[service-manager] Stopped")
	return nil
}
