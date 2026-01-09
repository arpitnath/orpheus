// Package config handles loading and validating agent.yaml configuration files.
package config

import "time"

// SessionConfig holds session affinity configuration for stateful agents.
// When enabled, requests with the same session ID are routed to the same worker
// when possible, enabling in-memory state continuity across requests.
type SessionConfig struct {
	Enabled     bool          `yaml:"enabled"`      // Enable session affinity (default: false)
	Key         string        `yaml:"key"`          // HTTP header name to extract session ID (default: X-Session-ID)
	TTL         time.Duration `yaml:"ttl"`          // Session memory duration (default: 30m)
	WaitTimeout time.Duration `yaml:"wait_timeout"` // Max wait for preferred worker (default: 100ms)
}

// SetDefaults applies default values to SessionConfig fields.
func (c *SessionConfig) SetDefaults() {
	if c.Key == "" {
		c.Key = DefaultSessionKey
	}
	if c.TTL == 0 {
		c.TTL = DefaultSessionTTL
	}
	if c.WaitTimeout == 0 {
		c.WaitTimeout = DefaultSessionWaitTimeout
	}
}

// AgentConfig represents the parsed agent.yaml configuration
type AgentConfig struct {
	// Required fields from YAML
	Name       string `yaml:"name"`
	Runtime    string `yaml:"runtime"`
	Module     string `yaml:"module"`
	Entrypoint string `yaml:"entrypoint"`

	// Optional fields from YAML
	InputType string   `yaml:"input_type,omitempty"`
	Env       []string `yaml:"env,omitempty"`

	// Memory configuration (Agent-Native: Graceful Degradation)
	Memory      int   `yaml:"memory,omitempty"`       // Target memory in MB (fast tier, soft limit)
	MemoryLimit int   `yaml:"memory_limit,omitempty"` // Hard limit in MB (with swap)
	SwapEnabled *bool `yaml:"swap_enabled,omitempty"` // Enable swap for graceful degradation (nil = default true)

	// Timeout configuration (Agent-Native: Activity-Based)
	TimeoutSec       int `yaml:"timeout,omitempty"`        // Max total time in seconds (hard limit)
	IdleTimeoutSec   int `yaml:"idle_timeout,omitempty"`   // No activity timeout in seconds
	ActivityCheckSec int `yaml:"activity_check,omitempty"` // Activity check interval in seconds

	// Session affinity configuration
	Session SessionConfig `yaml:"session,omitempty"`

	// Model configuration (NEW - ServiceManager)
	Model string `yaml:"model,omitempty"` // Model name (e.g., "mistral-7b", "openai:gpt-4")

	// Internal fields (not from YAML, computed at load time)
	AgentDir      string        `yaml:"-"`
	ConfigPath    string        `yaml:"-"`
	Timeout       time.Duration `yaml:"-"` // Computed from TimeoutSec
	IdleTimeout   time.Duration `yaml:"-"` // Computed from IdleTimeoutSec
	ActivityCheck time.Duration `yaml:"-"` // Computed from ActivityCheckSec
}

// Runtime type constants
const (
	RuntimePython3  = "python3"
	RuntimeNodeJS20 = "nodejs20" // Node.js 20 LTS with OpenAI Agents JS SDK support
)

// Default values
const (
	// Memory defaults (Agent-Native: Graceful Degradation)
	DefaultMemory      = 256 // MB - target (fast tier, soft limit)
	DefaultMemoryLimit = 512 // MB - hard limit (with swap enabled)
	DefaultSwapEnabled = true

	// Timeout defaults (Agent-Native: Activity-Based)
	DefaultTimeout       = 300 * time.Second // 5 min max total
	DefaultIdleTimeout   = 60 * time.Second  // 60s no activity = stuck
	DefaultActivityCheck = 5 * time.Second   // Check every 5s

	// Session affinity defaults
	DefaultSessionKey         = "X-Session-ID"        // HTTP header name
	DefaultSessionTTL         = 30 * time.Minute      // Session memory duration
	DefaultSessionWaitTimeout = 100 * time.Millisecond // Max wait for preferred worker

	// Runtime defaults
	DefaultRuntime = RuntimePython3
)
