package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads environment variables from a .env file.
//
// Format (standard .env):
//   KEY=value
//   KEY="quoted value"
//   KEY='single quoted'
//   # Comments are ignored
//   (empty lines ignored)
//
// Returns a map of environment variables or empty map if file doesn't exist.
func LoadDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Not an error if .env doesn't exist
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			// Skip malformed lines silently (lenient parsing)
			continue
		}

		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Strip surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		vars[name] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return vars, nil
}

// AutoLoadDotEnv loads .env from the agent directory if it exists.
// This is called automatically during config loading for local development convenience.
func AutoLoadDotEnv(agentDir string) (map[string]string, error) {
	dotEnvPath := filepath.Join(agentDir, ".env")
	return LoadDotEnv(dotEnvPath)
}
