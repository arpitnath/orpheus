package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"
)

// Manager manages model servers using actor model (race-free)
type Manager struct {
	commands      chan Command
	done          chan struct{}
	subscriptions []StateSubscription

	servers map[string]ModelServer
	state   ServerState
	backend ModelServer

	tokenBucket     *TokenBucket
	backoff         *BackoffCalculator
	processExitChan chan ExitResult
	healthEventChan chan HealthEvent
	process         *os.Process

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	pidFilePath string
}

// NewManager creates a new service manager
func NewManager() *Manager {
	return &Manager{
		servers:         make(map[string]ModelServer),
		commands:        make(chan Command, 10),
		done:            make(chan struct{}),
		state:           StateStopped,
		pidFilePath:     "/tmp/orpheus-model-server.lock",
		tokenBucket:     NewTokenBucket(5, 1.0/60.0),
		backoff:         NewBackoffCalculator(2*time.Second, 60*time.Second, 2.0, 0.2),
		processExitChan: make(chan ExitResult, 1),
		healthEventChan: make(chan HealthEvent, 1),
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

		case exit := <-m.processExitChan:
			m.handleProcessExit(exit)

		case event := <-m.healthEventChan:
			m.handleHealthEvent(event)

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

func (m *Manager) handleProcessExit(exit ExitResult) {
	log.Printf("[supervision] Process exited: code=%d", exit.ExitCode)

	if !m.tokenBucket.TryConsume() {
		log.Printf("[supervision] Circuit breaker open - stopping restart attempts")
		m.state = StateStopped
		return
	}

	delay := m.backoff.Next(exit.ExitCode)

	if delay == 0 {
		log.Printf("[supervision] Exit code %d - not restarting", exit.ExitCode)
		m.state = StateStopped
		return
	}

	log.Printf("[supervision] Restarting after %v backoff", delay)
	m.state = StateStarting

	select {
	case <-time.After(delay):
	case <-m.ctx.Done():
		return
	}

	if err := m.restartWithDoubleKill(m.ctx); err != nil {
		log.Printf("[supervision] Restart failed: %v", err)
		m.state = StateStopped
	}
}

func (m *Manager) handleHealthEvent(event HealthEvent) {
	if event.Type == HealthCheckPassed {
		log.Printf("[supervision] Service healthy - resetting supervision state")
		m.backoff.Reset()
		m.tokenBucket.Reset()
		m.state = StateReady
	} else {
		log.Printf("[supervision] Health check failed (%d consecutive) - restarting", event.Failures)

		m.state = StateStopped
		if err := m.restartWithDoubleKill(m.ctx); err != nil {
			log.Printf("[supervision] Health-triggered restart failed: %v", err)
		}
	}
}

func (m *Manager) restartWithDoubleKill(ctx context.Context) error {
	if m.backend == nil {
		return fmt.Errorf("no backend configured")
	}

	if m.process != nil {
		if err := killProcessGracefully(m.process, 5*time.Second); err != nil {
			log.Printf("[supervision] Force kill zombie: %v", err)
		}
		m.process = nil
	}

	port := 11434
	if !waitForPortFree(port, 5*time.Second) {
		return fmt.Errorf("port %d still occupied", port)
	}

	m.state = StateStarting
	m.notifySubscribers()

	if err := m.backend.Start(ctx); err != nil {
		m.state = StateStopped
		m.notifySubscribers()
		return err
	}

	if cmd := m.backend.GetCommand(); cmd != nil {
		m.process = m.backend.GetProcess()
		startProcessMonitor(cmd, m.processExitChan)
	}

	m.state = StateReady
	m.notifySubscribers()

	return nil
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

	if cmd := m.backend.GetCommand(); cmd != nil {
		m.process = m.backend.GetProcess()
		startProcessMonitor(cmd, m.processExitChan)
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
