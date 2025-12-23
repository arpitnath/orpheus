// Package client provides a client for the agentscale daemon.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"time"
)

// Client is the agentscale daemon client.
type Client struct {
	socketPath string
	httpClient *http.Client
}

// RunRequest is the request for running an agent.
type RunRequest struct {
	AgentPath string                 `json:"agent_path"`
	Input     map[string]interface{} `json:"input"`
	Options   RunOptions             `json:"options"`
}

// RunOptions are optional execution parameters.
type RunOptions struct {
	MemoryLimit int `json:"memory_limit"` // MB
	Timeout     int `json:"timeout"`      // seconds
	IdleTimeout int `json:"idle_timeout"` // seconds
}

// RunResponse is the response from running an agent.
type RunResponse struct {
	Status     string                 `json:"status"`
	Output     map[string]interface{} `json:"output,omitempty"`
	RawOutput  string                 `json:"raw_output,omitempty"`
	Error      string                 `json:"error,omitempty"`
	DurationMs int64                  `json:"duration_ms"`
}

// AgentStatus is the status of a running agent.
type AgentStatus struct {
	ID        string `json:"id"`
	AgentPath string `json:"agent_path"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at"`
	RunningMs int64  `json:"running_ms"`
}

// HealthStatus is the daemon health status.
type HealthStatus struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	RunningAgents int    `json:"running_agents"`
}

// NewClient creates a new daemon client.
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
			Timeout: 10 * time.Minute, // Long timeout for agent execution
		},
	}
}

// NewClientDefault creates a client with the default socket path.
func NewClientDefault() *Client {
	return NewClient(DefaultSocketPath())
}

// DefaultSocketPath returns the default daemon socket path.
func DefaultSocketPath() string {
	if goruntime.GOOS == "darwin" {
		// macOS: Lima-forwarded socket
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".lima", "agentscale", "sock", "agentscale.sock")
	}
	// Linux: Local socket
	return "/var/run/agentscale.sock"
}

// Run executes an agent.
func (c *Client) Run(ctx context.Context, req *RunRequest) (*RunResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/v1/agents/run", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var runResp RunResponse
	if err := json.Unmarshal(respBody, &runResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &runResp, nil
}

// Status gets the status of a running agent.
func (c *Client) Status(ctx context.Context, agentID string) (*AgentStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost/v1/agents/%s", agentID), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var status AgentStatus
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &status, nil
}

// Kill stops a running agent.
func (c *Client) Kill(ctx context.Context, agentID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("http://localhost/v1/agents/%s", agentID), nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	return nil
}

// Health checks the daemon health.
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/v1/health", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var health HealthStatus
	if err := json.Unmarshal(respBody, &health); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &health, nil
}

// IsRunning checks if the daemon is running.
func (c *Client) IsRunning(ctx context.Context) bool {
	_, err := c.Health(ctx)
	return err == nil
}

// SocketPath returns the socket path this client is using.
func (c *Client) SocketPath() string {
	return c.socketPath
}
