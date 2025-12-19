package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadServerConfig loads and validates agentscale.yaml configuration
func LoadServerConfig(configPath string) (*ServerConfig, error) {
	// 1. Resolve absolute path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to resolve config path", err)
	}

	// 2. Check file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, NewConfigError(ErrCodeNotFound, "config file not found: "+absPath)
	}

	// 3. Read and parse YAML
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, WrapError(ErrCodeFileRead, "failed to read config file", err)
	}

	var cfg ServerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, WrapError(ErrCodeInvalidYAML, "failed to parse config", err)
	}

	// 4. Load each agent's agent.yaml
	baseDir := filepath.Dir(absPath)
	for agentID, deployment := range cfg.Agents {
		var agentCfg *AgentConfig
		var agentPath string

		// Handle deployed image vs code directory
		if deployment.Image != "" {
			// Deployed agent: Load config from image manifest
			deployment.ImagePath = deployment.Image

			// Validate image exists
			if _, err := os.Stat(deployment.ImagePath); err != nil {
				return nil, fmt.Errorf("agent %s: deployed image not found at %s", agentID, deployment.ImagePath)
			}

			// Load agent.yaml from image (if exists) or create minimal config
			agentYamlPath := filepath.Join(deployment.ImagePath, "agent", "agent.yaml")
			if _, err := os.Stat(agentYamlPath); err == nil {
				agentCfg, err = LoadFromFile(agentYamlPath)
				if err != nil {
					return nil, fmt.Errorf("failed to load agent %s from image: %w", agentID, err)
				}
			} else {
				// Create minimal config for deployed agent
				agentCfg = &AgentConfig{
					Name:    agentID,
					Runtime: RuntimePython3,
				}
			}

			log.Printf("[config] Using deployed image for '%s': %s", agentID, deployment.ImagePath)
		} else if deployment.Path != "" {
			// Backward compat: code directory
			agentPath = deployment.Path
			if !filepath.IsAbs(agentPath) {
				agentPath = filepath.Join(baseDir, agentPath)
			}

			var err error
			agentCfg, err = Load(agentPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load agent %s from %s: %w", agentID, agentPath, err)
			}

			deployment.ImagePath = ""
			log.Printf("[config] Using code directory for '%s': %s", agentID, agentPath)
		} else {
			return nil, fmt.Errorf("agent %s: must specify either 'image' or 'path'", agentID)
		}

		deployment.AgentConfig = agentCfg
		cfg.Agents[agentID] = deployment
	}

	// 5. Apply defaults
	if err := applyServerDefaults(&cfg); err != nil {
		return nil, err
	}

	// 6. Validate
	if err := validateServerConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// applyServerDefaults applies default values to server configuration
func applyServerDefaults(cfg *ServerConfig) error {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	if cfg.Server.AutoscalerInterval == "" {
		cfg.Server.AutoscalerInterval = "10s"
	}

	// Validate autoscaler interval can be parsed
	if _, err := time.ParseDuration(cfg.Server.AutoscalerInterval); err != nil {
		return NewFieldError(ErrCodeInvalidValue, "server.autoscaler_interval",
			"invalid duration: "+cfg.Server.AutoscalerInterval)
	}

	// Apply isolation defaults if not set
	if cfg.Server.Isolation.Defaults.MemoryLimit == "" {
		cfg.Server.Isolation.Defaults.MemoryLimit = "512mb"
	}

	if cfg.Server.Isolation.Defaults.Timeout == "" {
		cfg.Server.Isolation.Defaults.Timeout = "300s"
	}

	return nil
}

// validateServerConfig validates entire server configuration
func validateServerConfig(cfg *ServerConfig) error {
	// Validate port
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return NewFieldError(ErrCodeInvalidValue, "server.port",
			"port must be between 1 and 65535")
	}

	// Validate autoscaler interval
	interval, err := time.ParseDuration(cfg.Server.AutoscalerInterval)
	if err != nil {
		return NewFieldError(ErrCodeInvalidValue, "server.autoscaler_interval",
			"invalid duration: "+cfg.Server.AutoscalerInterval)
	}
	if interval <= 0 {
		return NewFieldError(ErrCodeInvalidValue, "server.autoscaler_interval",
			"must be positive")
	}

	// Validate isolation defaults
	if err := validateIsolationDefaults(cfg.Server.Isolation.Defaults); err != nil {
		return fmt.Errorf("server.isolation.defaults: %w", err)
	}

	// Validate at least one agent
	if len(cfg.Agents) == 0 {
		return NewConfigError(ErrCodeMissingField, "no agents configured")
	}

	// Validate each agent
	for agentID, deployment := range cfg.Agents {
		if deployment.AgentConfig == nil {
			return fmt.Errorf("agent %s: config not loaded", agentID)
		}

		if err := validateAgentDeployment(agentID, deployment); err != nil {
			return err
		}
	}

	return nil
}

// validateAgentDeployment validates a single agent's deployment configuration
func validateAgentDeployment(agentID string, deployment AgentDeployment) error {
	sc := deployment.Scaling

	// Validate worker counts
	if sc.MinWorkers < 0 {
		return fmt.Errorf("agent %s: min_workers must be >= 0", agentID)
	}
	if sc.MaxWorkers <= sc.MinWorkers {
		return fmt.Errorf("agent %s: max_workers (%d) must be > min_workers (%d)",
			agentID, sc.MaxWorkers, sc.MinWorkers)
	}

	// Validate thresholds
	if sc.ScaleUpThreshold <= sc.TargetUtilization {
		return fmt.Errorf("agent %s: scale_up_threshold (%.2f) must be > target_utilization (%.2f)",
			agentID, sc.ScaleUpThreshold, sc.TargetUtilization)
	}
	if sc.ScaleDownThreshold >= sc.TargetUtilization {
		return fmt.Errorf("agent %s: scale_down_threshold (%.2f) must be < target_utilization (%.2f)",
			agentID, sc.ScaleDownThreshold, sc.TargetUtilization)
	}

	// Validate queue size
	if sc.QueueSize <= 0 {
		return fmt.Errorf("agent %s: queue_size must be positive", agentID)
	}

	// Validate delays can be parsed
	if _, err := time.ParseDuration(sc.ScaleUpDelay); err != nil {
		return fmt.Errorf("agent %s: invalid scale_up_delay '%s': %w",
			agentID, sc.ScaleUpDelay, err)
	}
	if _, err := time.ParseDuration(sc.ScaleDownDelay); err != nil {
		return fmt.Errorf("agent %s: invalid scale_down_delay '%s': %w",
			agentID, sc.ScaleDownDelay, err)
	}

	// Validate isolation override if present
	if deployment.Isolation != nil {
		if deployment.Isolation.MemoryLimit != "" {
			if _, err := parseMemoryLimit(deployment.Isolation.MemoryLimit); err != nil {
				return fmt.Errorf("agent %s: isolation.memory_limit: %w", agentID, err)
			}
		}
		if deployment.Isolation.Timeout != "" {
			if _, err := time.ParseDuration(deployment.Isolation.Timeout); err != nil {
				return fmt.Errorf("agent %s: isolation.timeout: invalid duration: %w", agentID, err)
			}
		}
	}

	return nil
}

// validateIsolationDefaults validates isolation default settings
func validateIsolationDefaults(defaults IsolationDefaults) error {
	// Validate memory limit
	if _, err := parseMemoryLimit(defaults.MemoryLimit); err != nil {
		return fmt.Errorf("memory_limit: %w", err)
	}

	// Validate timeout
	if _, err := time.ParseDuration(defaults.Timeout); err != nil {
		return fmt.Errorf("timeout: invalid duration: %w", err)
	}

	return nil
}

// ParseScalingConfig converts YAML scaling config to parsed config with durations
func ParseScalingConfig(sc ScalingConfig) (ParsedScalingConfig, error) {
	scaleUpDelay, err := time.ParseDuration(sc.ScaleUpDelay)
	if err != nil {
		return ParsedScalingConfig{}, fmt.Errorf("invalid scale_up_delay: %w", err)
	}

	scaleDownDelay, err := time.ParseDuration(sc.ScaleDownDelay)
	if err != nil {
		return ParsedScalingConfig{}, fmt.Errorf("invalid scale_down_delay: %w", err)
	}

	return ParsedScalingConfig{
		MinWorkers:         sc.MinWorkers,
		MaxWorkers:         sc.MaxWorkers,
		TargetUtilization:  sc.TargetUtilization,
		ScaleUpThreshold:   sc.ScaleUpThreshold,
		ScaleDownThreshold: sc.ScaleDownThreshold,
		ScaleUpDelay:       scaleUpDelay,
		ScaleDownDelay:     scaleDownDelay,
		QueueSize:          sc.QueueSize,
		IdleTimeout:        10 * time.Minute, // Fixed for now
	}, nil
}

// ParseIsolationDefaults parses isolation defaults with memory/timeout strings
func ParseIsolationDefaults(defaults IsolationDefaults, override *IsolationOverride) (ParsedIsolationDefaults, error) {
	// Start with defaults
	memLimit := defaults.MemoryLimit
	timeout := defaults.Timeout

	// Apply overrides if present
	if override != nil {
		if override.MemoryLimit != "" {
			memLimit = override.MemoryLimit
		}
		if override.Timeout != "" {
			timeout = override.Timeout
		}
	}

	// Parse memory limit
	memLimitMB, err := parseMemoryLimit(memLimit)
	if err != nil {
		return ParsedIsolationDefaults{}, err
	}

	// Parse timeout
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		return ParsedIsolationDefaults{}, fmt.Errorf("invalid timeout: %w", err)
	}

	return ParsedIsolationDefaults{
		Enabled:     true, // Always enabled in multi-agent mode
		Type:        "auto",
		MemoryLimit: memLimitMB,
		Timeout:     timeoutDuration,
	}, nil
}

// parseMemoryLimit parses memory limit string like "512mb" or "1gb" to MB
func parseMemoryLimit(limit string) (int, error) {
	limit = strings.ToLower(strings.TrimSpace(limit))
	if limit == "" {
		return 0, fmt.Errorf("memory limit cannot be empty")
	}

	// Check for MB suffix
	if strings.HasSuffix(limit, "mb") {
		mbStr := strings.TrimSuffix(limit, "mb")
		mb, err := strconv.Atoi(mbStr)
		if err != nil {
			return 0, fmt.Errorf("invalid memory limit '%s': %w", limit, err)
		}
		if mb <= 0 {
			return 0, fmt.Errorf("memory limit must be positive")
		}
		return mb, nil
	}

	// Check for GB suffix
	if strings.HasSuffix(limit, "gb") {
		gbStr := strings.TrimSuffix(limit, "gb")
		gb, err := strconv.Atoi(gbStr)
		if err != nil {
			return 0, fmt.Errorf("invalid memory limit '%s': %w", limit, err)
		}
		if gb <= 0 {
			return 0, fmt.Errorf("memory limit must be positive")
		}
		return gb * 1024, nil // Convert to MB
	}

	return 0, fmt.Errorf("memory limit must end with 'mb' or 'gb', got: %s", limit)
}
