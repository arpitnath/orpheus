package service

import (
	"context"
	"testing"
	"time"
)

func TestNewVLLMServer(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		mode     ServerMode
		endpoint string
		want     string
	}{
		{
			name:     "default endpoint",
			model:    "mistral",
			mode:     ServerModeManaged,
			endpoint: "",
			want:     "http://localhost:8000",
		},
		{
			name:     "custom endpoint",
			model:    "llama-3-8b",
			mode:     ServerModeExternal,
			endpoint: "http://remote:8000",
			want:     "http://remote:8000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewVLLMServer(tt.model, tt.mode, tt.endpoint)

			if server.Endpoint() != tt.want {
				t.Errorf("Endpoint() = %v, want %v", server.Endpoint(), tt.want)
			}

			if server.Mode() != tt.mode {
				t.Errorf("Mode() = %v, want %v", server.Mode(), tt.mode)
			}

			if server.modelName != tt.model {
				t.Errorf("modelName = %v, want %v", server.modelName, tt.model)
			}

			if server.state != StateStopped {
				t.Errorf("initial state = %v, want %v", server.state, StateStopped)
			}
		})
	}
}

func TestVLLMServerStatus(t *testing.T) {
	server := NewVLLMServer("test-model", ServerModeManaged, "http://localhost:8000")

	status := server.Status()

	if status.State != StateStopped {
		t.Errorf("Status().State = %v, want %v", status.State, StateStopped)
	}

	if status.Endpoint != "http://localhost:8000" {
		t.Errorf("Status().Endpoint = %v, want %v", status.Endpoint, "http://localhost:8000")
	}

	if status.Model != "test-model" {
		t.Errorf("Status().Model = %v, want %v", status.Model, "test-model")
	}
}

func TestVLLMServerGetProcess(t *testing.T) {
	server := NewVLLMServer("test-model", ServerModeManaged, "")

	proc := server.GetProcess()
	if proc != nil {
		t.Errorf("GetProcess() = %v, want nil (no process started)", proc)
	}

	cmd := server.GetCommand()
	if cmd != nil {
		t.Errorf("GetCommand() = %v, want nil (no command started)", cmd)
	}
}

func TestVLLMServerExternalMode(t *testing.T) {
	server := NewVLLMServer("test-model", ServerModeExternal, "http://nonexistent:8000")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := server.Start(ctx)

	if err == nil {
		t.Error("Start() should fail for unreachable external server")
	}

	if server.state != StateStopped {
		t.Errorf("state after failed start = %v, want %v", server.state, StateStopped)
	}
}

func TestVLLMServerHealthCheck(t *testing.T) {
	server := NewVLLMServer("test-model", ServerModeManaged, "http://nonexistent:8000")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	healthy, err := server.Health(ctx)

	if healthy {
		t.Error("Health() should return false for nonexistent server")
	}

	if err == nil {
		t.Error("Health() should return error for unreachable server")
	}
}
