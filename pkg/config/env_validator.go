package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Blocklist of dangerous environment variables that could compromise container security.
// These are either controlled by the system or could be used for privilege escalation.
var forbiddenEnvVars = map[string]bool{
	// Dynamic linker variables (library injection attacks)
	"LD_PRELOAD":       true,
	"LD_LIBRARY_PATH":  true,
	"LD_AUDIT":         true,

	// System-controlled paths (prevent hijacking)
	"PATH":       true, // System sets this for consistent behavior
	"PYTHONPATH": true, // System sets this for agent isolation
	"HOME":       true, // System sets this to /tmp

	// Shell and execution environment
	"SHELL":    true, // Shell override
	"IFS":      true, // Input field separator manipulation
	"CDPATH":   true, // Directory traversal
	"ENV":      true, // POSIX shell init
	"BASH_ENV": true, // Bash init

	// Python-specific dangerous vars
	"PYTHONHOME":     true, // Python installation hijacking
	"PYTHONSTARTUP":  true, // Execute code at Python start
	"PYTHONWARNINGS": true, // Could hide security warnings
}

// Valid environment variable name regex (POSIX standard)
// Must start with letter or underscore, contain only alphanumeric and underscore
var validEnvNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateEnvVars validates all environment variables in the agent configuration.
// Checks:
//   - Format (KEY=value)
//   - Name validity (POSIX compliance)
//   - Security blocklist
//   - Size limits
func ValidateEnvVars(envs []string) error {
	if len(envs) > 128 {
		return fmt.Errorf("too many environment variables: %d (maximum 128 allowed)", len(envs))
	}

	totalSize := 0
	for i, env := range envs {
		if err := validateEnvVar(env); err != nil {
			return fmt.Errorf("env[%d]: %w", i, err)
		}
		totalSize += len(env)
	}

	if totalSize > 1024*1024 {
		return fmt.Errorf("total environment size exceeds 1MB limit")
	}

	return nil
}

// validateEnvVar validates a single environment variable.
func validateEnvVar(env string) error {
	// Parse KEY=value format
	parts := strings.SplitN(env, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: %q (expected KEY=value)", env)
	}

	name := parts[0]
	value := parts[1]

	// Validate name format (POSIX compliance)
	if !validEnvNameRegex.MatchString(name) {
		return fmt.Errorf("invalid variable name: %q (must start with letter/underscore, contain only alphanumeric/underscore)", name)
	}

	// Check security blocklist
	if forbiddenEnvVars[strings.ToUpper(name)] {
		return fmt.Errorf("forbidden variable: %q (system-reserved for security)", name)
	}

	// Size limits
	if len(name) > 256 {
		return fmt.Errorf("variable name too long: %q (maximum 256 characters)", name)
	}

	if len(value) > 65536 {
		return fmt.Errorf("variable value too long: %q (maximum 64KB)", name)
	}

	// Security: reject null bytes (string terminators)
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("variable value contains null byte: %q", name)
	}

	return nil
}
