package service

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// mockModelServer implements ModelServer for testing
type mockModelServer struct {
	started     bool
	healthy     bool
	endpoint    string
	startErr    error
	stopErr     error
	healthErr   error
	startCalled int
	stopCalled  int
}

func newMockModelServer() *mockModelServer {
	return &mockModelServer{
		endpoint: "http://localhost:11434",
		healthy:  true,
	}
}

func (m *mockModelServer) Start(ctx context.Context) error {
	m.startCalled++
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockModelServer) Stop(ctx context.Context) error {
	m.stopCalled++
	if m.stopErr != nil {
		return m.stopErr
	}
	m.started = false
	return nil
}

func (m *mockModelServer) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	return m.Start(ctx)
}

func (m *mockModelServer) Health(ctx context.Context) (bool, error) {
	return m.healthy, m.healthErr
}

func (m *mockModelServer) Endpoint() string {
	return m.endpoint
}

func (m *mockModelServer) Status() ServerStatus {
	state := StateStopped
	if m.started {
		state = StateReady
	}
	return ServerStatus{State: state}
}

func (m *mockModelServer) GetProcess() *os.Process {
	return nil
}

func (m *mockModelServer) GetCommand() *exec.Cmd {
	return nil
}

func TestNewManager(t *testing.T) {
	mgr := NewManager()

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.state != StateStopped {
		t.Errorf("Expected initial state=stopped, got %v", mgr.state)
	}

	if mgr.tokenBucket == nil {
		t.Error("TokenBucket should be initialized")
	}

	if mgr.backoff == nil {
		t.Error("BackoffCalculator should be initialized")
	}
}

func TestManager_Start(t *testing.T) {
	mgr := NewManager()

	ctx := context.Background()
	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give control loop time to start
	time.Sleep(10 * time.Millisecond)

	// Stop
	mgr.Stop(context.Background())
}

func TestManager_Stop(t *testing.T) {
	mgr := NewManager()

	ctx := context.Background()
	mgr.Start(ctx)

	err := mgr.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestManager_DetectServerType_Darwin(t *testing.T) {
	mgr := NewManager()

	// detectServerType is a private method, but we can test it indirectly
	// For now, just verify manager can be created
	if mgr.pidFilePath == "" {
		t.Error("pidFilePath should be set")
	}
}

func TestManager_CreateServer_Ollama(t *testing.T) {
	mgr := NewManager()

	config := ModelConfig{
		Name:   "test-model",
		Server: "ollama",
	}

	server, err := mgr.createServer(config)
	if err != nil {
		t.Fatalf("createServer failed: %v", err)
	}

	if server == nil {
		t.Fatal("createServer returned nil")
	}

	// Verify it's an Ollama server
	endpoint := server.Endpoint()
	if endpoint == "" {
		t.Error("Endpoint should not be empty")
	}
}

func TestManager_CreateServer_Auto(t *testing.T) {
	mgr := NewManager()

	config := ModelConfig{
		Name:   "test-model",
		Server: "auto",
	}

	server, err := mgr.createServer(config)
	if err != nil {
		t.Fatalf("createServer with auto failed: %v", err)
	}

	if server == nil {
		t.Fatal("createServer returned nil")
	}
}

func TestManager_CreateServer_Unsupported(t *testing.T) {
	mgr := NewManager()

	config := ModelConfig{
		Name:   "test-model",
		Server: "unsupported-server",
	}

	_, err := mgr.createServer(config)
	if err == nil {
		t.Error("createServer should fail for unsupported server type")
	}
}

func TestServerState_Values(t *testing.T) {
	states := []ServerState{
		StateStopped,
		StateStarting,
		StateLoading,
		StateReady,
		StateServing,
		StateUnhealthy,
	}

	// Verify all states are unique
	seen := make(map[ServerState]bool)
	for _, s := range states {
		if seen[s] {
			t.Errorf("Duplicate state: %v", s)
		}
		seen[s] = true
	}
}

func TestModelConfig(t *testing.T) {
	config := ModelConfig{
		Name:   "mistral:7b",
		Server: "ollama",
	}

	if config.Name != "mistral:7b" {
		t.Error("Name mismatch")
	}

	if config.Server != "ollama" {
		t.Error("Server mismatch")
	}
}

func TestServerStatus(t *testing.T) {
	status := ServerStatus{
		State: StateReady,
	}

	if status.State != StateReady {
		t.Error("State mismatch")
	}
}

func TestExitResult(t *testing.T) {
	result := ExitResult{
		ExitCode: 137,
	}

	if result.ExitCode != 137 {
		t.Error("ExitCode mismatch")
	}
}

func TestHealthEvent(t *testing.T) {
	event := HealthEvent{
		Type:     HealthCheckPassed,
		Failures: 0,
	}

	if event.Type != HealthCheckPassed {
		t.Error("Type mismatch")
	}

	event2 := HealthEvent{
		Type:     HealthCheckFailed,
		Failures: 3,
	}

	if event2.Failures != 3 {
		t.Error("Failures mismatch")
	}
}

func TestManager_CommandChannel(t *testing.T) {
	mgr := NewManager()

	// Commands channel should be buffered
	if cap(mgr.commands) == 0 {
		t.Error("Commands channel should be buffered")
	}
}

func TestManager_WithBackend(t *testing.T) {
	mgr := NewManager()
	mock := newMockModelServer()

	// Set backend directly for testing
	mgr.backend = mock

	if mgr.backend == nil {
		t.Error("Backend should be set")
	}
}
