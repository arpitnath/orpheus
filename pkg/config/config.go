// Package config handles loading and validating agent.yaml configuration files.
package config

import "time"

// AgentConfig represents the parsed agent.yaml configuration
type AgentConfig struct {
	// Required fields from YAML
	Name       string `yaml:"name"`
	Runtime    string `yaml:"runtime"`
	Module     string `yaml:"module"`
	Entrypoint string `yaml:"entrypoint"`

	// Optional fields from YAML
	InputType  string   `yaml:"input_type,omitempty"`
	Memory     int      `yaml:"memory,omitempty"`
	TimeoutSec int      `yaml:"timeout,omitempty"` // Timeout in seconds from YAML
	Env        []string `yaml:"env,omitempty"`

	// Internal fields (not from YAML)
	AgentDir   string        `yaml:"-"`
	ConfigPath string        `yaml:"-"`
	Timeout    time.Duration `yaml:"-"` // Computed from TimeoutSec
}

// Runtime type constants
const (
	RuntimePython3 = "python3"
	RuntimeNode    = "node" // Future support
)

// Default values
const (
	DefaultMemory  = 512 // MB
	DefaultTimeout = 60 * time.Second
	DefaultRuntime = RuntimePython3
)
