// Package config handles loading and validating agent.yaml configuration files.
package config

import "time"

// ApplyDefaults sets default values for unspecified optional fields
func ApplyDefaults(cfg *AgentConfig) {
	// Set default runtime if not specified
	if cfg.Runtime == "" {
		cfg.Runtime = DefaultRuntime
	}

	// ============================================
	// Memory defaults (Agent-Native: Graceful Degradation)
	// ============================================

	// Set default memory target (soft limit) if not specified
	if cfg.Memory == 0 {
		cfg.Memory = DefaultMemory
	}

	// Set default memory limit (hard limit) if not specified
	if cfg.MemoryLimit == 0 {
		cfg.MemoryLimit = DefaultMemoryLimit
	}

	// Ensure memory limit is >= memory target
	if cfg.MemoryLimit < cfg.Memory {
		cfg.MemoryLimit = cfg.Memory * 2 // Double the target if limit is lower
	}

	// SwapEnabled defaults to true for Agent-Native graceful degradation
	// Using pointer allows distinguishing "not set" (nil) from explicit false
	if cfg.SwapEnabled == nil {
		defaultSwap := DefaultSwapEnabled
		cfg.SwapEnabled = &defaultSwap
	}

	// ============================================
	// Timeout defaults (Agent-Native: Activity-Based)
	// ============================================

	// Convert TimeoutSec to Timeout duration (max total time)
	if cfg.TimeoutSec > 0 {
		cfg.Timeout = time.Duration(cfg.TimeoutSec) * time.Second
	} else {
		cfg.Timeout = DefaultTimeout
	}

	// Convert IdleTimeoutSec to IdleTimeout duration (no activity timeout)
	if cfg.IdleTimeoutSec > 0 {
		cfg.IdleTimeout = time.Duration(cfg.IdleTimeoutSec) * time.Second
	} else {
		cfg.IdleTimeout = DefaultIdleTimeout
	}

	// Convert ActivityCheckSec to ActivityCheck duration (check interval)
	if cfg.ActivityCheckSec > 0 {
		cfg.ActivityCheck = time.Duration(cfg.ActivityCheckSec) * time.Second
	} else {
		cfg.ActivityCheck = DefaultActivityCheck
	}
}
