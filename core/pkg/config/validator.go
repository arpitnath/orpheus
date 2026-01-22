// Package config handles loading and validating agent.yaml configuration files.
package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// validLabelKeyRegex matches Prometheus label key format
	validLabelKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	// reservedLabelKeys cannot be used as custom labels
	reservedLabelKeys = map[string]bool{
		"agent":    true, // System-managed
		"le":       true, // Histogram bucket
		"quantile": true, // Summary quantile
		"engine":   true, // Service collector uses this
		"status":   true, // ExecLog collector uses this
	}
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

	// Validate environment variables
	if len(cfg.Env) > 0 {
		if err := ValidateEnvVars(cfg.Env); err != nil {
			return NewFieldError(ErrCodeInvalidValue, "env", err.Error())
		}
	}

	// Validate engine if specified
	if cfg.Engine != "" {
		if err := validateEngine(cfg.Engine); err != nil {
			return err
		}
	}

	// Validate telemetry labels
	if len(cfg.Telemetry.Labels) > 0 {
		if err := validateTelemetryLabels(cfg.Telemetry.Labels); err != nil {
			return err
		}
	}

	return nil
}

// validateRuntime checks if the runtime is supported
func validateRuntime(runtime string) error {
	switch runtime {
	case RuntimePython3, RuntimeNodeJS20:
		return nil
	default:
		return NewFieldError(ErrCodeUnsupportedRT, "runtime", "unsupported runtime: "+runtime+". Supported: python3, nodejs20")
	}
}

// validateModuleExists checks if the module file exists in the agent directory
func validateModuleExists(cfg *AgentConfig) error {
	// Build full path to module
	modulePath := filepath.Join(cfg.AgentDir, cfg.Module)

	// Determine extensions to try based on runtime
	var extensions []string
	switch cfg.Runtime {
	case RuntimeNodeJS20:
		extensions = []string{".js", ".mjs", ""}
	default: // Python
		extensions = []string{".py", ""}
	}

	// Check at root level first
	for _, ext := range extensions {
		checkPath := modulePath
		if ext != "" && !strings.HasSuffix(checkPath, ext) {
			checkPath = checkPath + ext
		}
		if _, err := os.Stat(checkPath); err == nil {
			return nil
		}
	}

	// If not found, try agent/ subdirectory (new deploy structure)
	modulePathInAgent := filepath.Join(cfg.AgentDir, "agent", cfg.Module)
	for _, ext := range extensions {
		checkPath := modulePathInAgent
		if ext != "" && !strings.HasSuffix(checkPath, ext) {
			checkPath = checkPath + ext
		}
		if _, err := os.Stat(checkPath); err == nil {
			return nil
		}
	}

	return NewFieldError(ErrCodeNotFound, "module", "module file not found: "+cfg.Module)
}

// validateEngine checks if the inference engine is supported
func validateEngine(engine string) error {
	validEngines := []string{"ollama", "vllm"}

	for _, valid := range validEngines {
		if engine == valid {
			return nil
		}
	}

	return NewFieldError(ErrCodeInvalidValue, "engine",
		"unsupported engine: "+engine+". Supported: ollama, vllm")
}

// validateTelemetryLabels validates custom label keys and values.
func validateTelemetryLabels(labels map[string]string) error {
	for key, value := range labels {
		// Check key format
		if !validLabelKeyRegex.MatchString(key) {
			return NewFieldError(ErrCodeInvalidValue, "telemetry.labels."+key,
				"label key must match [a-zA-Z_][a-zA-Z0-9_]*")
		}

		// Check reserved keys
		if reservedLabelKeys[key] {
			return NewFieldError(ErrCodeInvalidValue, "telemetry.labels."+key,
				"'"+key+"' is a reserved label name")
		}

		// Check value length
		if len(value) > 128 {
			return NewFieldError(ErrCodeInvalidValue, "telemetry.labels."+key,
				"label value exceeds 128 characters")
		}

		// Check for empty value
		if value == "" {
			return NewFieldError(ErrCodeInvalidValue, "telemetry.labels."+key,
				"label value cannot be empty")
		}
	}

	// Check total label count
	if len(labels) > 10 {
		return NewFieldError(ErrCodeInvalidValue, "telemetry.labels",
			"maximum 10 custom labels allowed")
	}

	return nil
}
