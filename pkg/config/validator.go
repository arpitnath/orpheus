// Package config handles loading and validating agent.yaml configuration files.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Validate checks the configuration for required fields and valid values
func Validate(cfg *AgentConfig) error {
	// Check required fields
	if strings.TrimSpace(cfg.Name) == "" {
		return NewFieldError(ErrCodeMissingField, "name", "agent name is required")
	}

	if strings.TrimSpace(cfg.Runtime) == "" {
		return NewFieldError(ErrCodeMissingField, "runtime", "runtime is required")
	}

	if strings.TrimSpace(cfg.Module) == "" {
		return NewFieldError(ErrCodeMissingField, "module", "module is required")
	}

	if strings.TrimSpace(cfg.Entrypoint) == "" {
		return NewFieldError(ErrCodeMissingField, "entrypoint", "entrypoint is required")
	}

	// Validate runtime
	if err := validateRuntime(cfg.Runtime); err != nil {
		return err
	}

	// Validate module file exists (if AgentDir is set)
	if cfg.AgentDir != "" {
		if err := validateModuleExists(cfg); err != nil {
			return err
		}
	}

	// Validate memory is positive if set
	if cfg.Memory < 0 {
		return NewFieldError(ErrCodeInvalidValue, "memory", "memory must be a positive value")
	}

	// Validate timeout is positive if set
	if cfg.TimeoutSec < 0 {
		return NewFieldError(ErrCodeInvalidValue, "timeout", "timeout must be a positive value")
	}

	return nil
}

// validateRuntime checks if the runtime is supported
func validateRuntime(runtime string) error {
	switch runtime {
	case RuntimePython3:
		return nil
	case RuntimeNode:
		// Node.js support is planned but not yet implemented
		return NewFieldError(ErrCodeUnsupportedRT, "runtime", "node runtime is not yet supported")
	default:
		return NewFieldError(ErrCodeUnsupportedRT, "runtime", "unsupported runtime: "+runtime+". Supported: python3")
	}
}

// validateModuleExists checks if the module file exists in the agent directory
func validateModuleExists(cfg *AgentConfig) error {
	// Build full path to module
	modulePath := filepath.Join(cfg.AgentDir, cfg.Module)

	// Check if it's a Python file (add .py extension if needed)
	if !strings.HasSuffix(modulePath, ".py") {
		modulePath = modulePath + ".py"
	}

	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return NewFieldError(ErrCodeNotFound, "module", "module file not found: "+cfg.Module)
	}

	return nil
}
