package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Variable reference patterns for env var substitution
var (
	// ${VAR} - simple reference, error if undefined
	simpleRefRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

	// ${VAR:-default} - reference with default value
	defaultRefRegex = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*):-(.*?)\}`)
)

// ResolveEnvReferences resolves ${VAR} and ${VAR:-default} references in environment variables.
//
// Sources are checked in this precedence order:
//   1. Runtime overrides (from CLI --env or API request)
//   2. Env file values (from --env-file or .env)
//   3. System environment variables (from shell)
//
// Returns resolved env vars in KEY=value format or error if required variables are undefined.
func ResolveEnvReferences(
	envVars []string,
	runtimeOverrides map[string]string,
	envFileVars map[string]string,
) ([]string, error) {
	resolved := make([]string, 0, len(envVars))

	for _, env := range envVars {
		// Parse KEY=value
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			// Skip malformed entries (should be caught by validation)
			continue
		}

		name := parts[0]
		value := parts[1]

		// Resolve any variable references in the value
		resolvedValue, err := resolveValue(value, runtimeOverrides, envFileVars)
		if err != nil {
			return nil, fmt.Errorf("environment variable %q: %w", name, err)
		}

		resolved = append(resolved, name+"="+resolvedValue)
	}

	return resolved, nil
}

// resolveValue resolves variable references in a single value string.
func resolveValue(
	value string,
	runtimeOverrides map[string]string,
	envFileVars map[string]string,
) (string, error) {
	// First, resolve ${VAR:-default} references (these have fallback values)
	value = defaultRefRegex.ReplaceAllStringFunc(value, func(match string) string {
		parts := defaultRefRegex.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}

		varName := parts[1]
		defaultVal := parts[2]

		// Check sources in precedence order
		if val, ok := runtimeOverrides[varName]; ok {
			return val
		}
		if val, ok := envFileVars[varName]; ok {
			return val
		}
		if val := os.Getenv(varName); val != "" {
			return val
		}

		// Variable not found - use default value
		return defaultVal
	})

	// Then, resolve ${VAR} references (these MUST be defined)
	unresolved := []string{}
	value = simpleRefRegex.ReplaceAllStringFunc(value, func(match string) string {
		parts := simpleRefRegex.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		varName := parts[1]

		// Check sources in precedence order
		if val, ok := runtimeOverrides[varName]; ok {
			return val
		}
		if val, ok := envFileVars[varName]; ok {
			return val
		}
		if val := os.Getenv(varName); val != "" {
			return val
		}

		// Variable not found and no default - track for error
		unresolved = append(unresolved, varName)
		return ""
	})

	// Error if any required variables were undefined
	if len(unresolved) > 0 {
		return "", fmt.Errorf("undefined variables: %v (use ${VAR:-default} to provide a default)", unresolved)
	}

	return value, nil
}
