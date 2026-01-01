// Package config handles loading and validating agent.yaml configuration files.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configFileName = "agent.yaml"

// Load reads and parses the agent.yaml file from the specified directory
func Load(agentDir string) (*AgentConfig, error) {
	// Resolve absolute path
	absDir, err := filepath.Abs(agentDir)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to resolve agent directory path", err)
	}

	// Check if directory exists
	info, err := os.Stat(absDir)
	if os.IsNotExist(err) {
		return nil, NewConfigError(ErrCodeNotFound, "agent directory does not exist: "+absDir)
	}
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to access agent directory", err)
	}
	if !info.IsDir() {
		return nil, NewConfigError(ErrCodeNotFound, "path is not a directory: "+absDir)
	}

	// Build config file path
	configPath := filepath.Join(absDir, configFileName)

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, NewConfigError(ErrCodeNotFound, "agent.yaml not found in: "+absDir)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to read agent.yaml", err)
	}

	// Parse YAML
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, WrapError(ErrCodeInvalidYAML, "failed to parse agent.yaml", err)
	}

	// Set internal fields
	cfg.AgentDir = absDir
	cfg.ConfigPath = configPath

	// Apply defaults
	ApplyDefaults(&cfg)

	// Auto-load .env file from agent directory (if it exists)
	dotEnvVars, err := AutoLoadDotEnv(absDir)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to load .env file", err)
	}

	// Resolve environment variable references (${VAR} and ${VAR:-default})
	// Runtime overrides will be empty here (only used during execution)
	if len(cfg.Env) > 0 {
		resolved, err := ResolveEnvReferences(cfg.Env, nil, dotEnvVars)
		if err != nil {
			return nil, WrapError(ErrCodeInvalidValue, "failed to resolve env var references", err)
		}
		cfg.Env = resolved
	}

	// Validate configuration
	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadFromFile reads and parses a specific config file path
func LoadFromFile(configPath string) (*AgentConfig, error) {
	// Resolve absolute path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to resolve config file path", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, NewConfigError(ErrCodeNotFound, "config file not found: "+absPath)
	}

	// Read config file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to read config file", err)
	}

	// Parse YAML
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, WrapError(ErrCodeInvalidYAML, "failed to parse config file", err)
	}

	// Set internal fields
	cfg.AgentDir = filepath.Dir(absPath)
	cfg.ConfigPath = absPath

	// Apply defaults
	ApplyDefaults(&cfg)

	// Validate configuration
	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
