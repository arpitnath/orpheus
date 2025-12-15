// Package config handles loading and validating agent.yaml configuration files.
package config

import "time"

// ApplyDefaults sets default values for unspecified optional fields
func ApplyDefaults(cfg *AgentConfig) {
	// Set default runtime if not specified
	if cfg.Runtime == "" {
		cfg.Runtime = DefaultRuntime
	}

	// Set default memory if not specified or zero
	if cfg.Memory == 0 {
		cfg.Memory = DefaultMemory
	}

	// Convert TimeoutSec to Timeout duration
	if cfg.TimeoutSec > 0 {
		cfg.Timeout = time.Duration(cfg.TimeoutSec) * time.Second
	} else {
		cfg.Timeout = DefaultTimeout
	}
}
