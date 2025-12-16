package config

import "time"

// ServerConfig represents the complete agentscale.yaml configuration
type ServerConfig struct {
	Server ServerSection                `yaml:"server"`
	Agents map[string]AgentDeployment   `yaml:"agents"`
}

// ServerSection contains server-level configuration
type ServerSection struct {
	Port               int              `yaml:"port"`
	AutoscalerInterval string           `yaml:"autoscaler_interval"`
	Isolation          IsolationSection `yaml:"isolation"`
}

// IsolationSection contains default isolation settings
type IsolationSection struct {
	Enabled  bool              `yaml:"enabled"`
	Type     string            `yaml:"type"` // "auto", "namespace", "vm"
	Defaults IsolationDefaults `yaml:"defaults"`
}

// IsolationDefaults applied to all agents unless overridden
type IsolationDefaults struct {
	MemoryLimit string `yaml:"memory_limit"` // "512mb", "1gb"
	Timeout     string `yaml:"timeout"`      // "300s", "5m"
}

// AgentDeployment represents one agent's deployment config
type AgentDeployment struct {
	Path      string             `yaml:"path"`
	Scaling   ScalingConfig      `yaml:"scaling"`
	Isolation *IsolationOverride `yaml:"isolation,omitempty"`

	// Internal (populated during load)
	AgentConfig *AgentConfig `yaml:"-"`
}

// ScalingConfig defines scaling behavior for an agent
type ScalingConfig struct {
	MinWorkers         int     `yaml:"min_workers"`
	MaxWorkers         int     `yaml:"max_workers"`
	TargetUtilization  float64 `yaml:"target_utilization"`
	ScaleUpThreshold   float64 `yaml:"scale_up_threshold"`
	ScaleDownThreshold float64 `yaml:"scale_down_threshold"`
	ScaleUpDelay       string  `yaml:"scale_up_delay"`
	ScaleDownDelay     string  `yaml:"scale_down_delay"`
	QueueSize          int     `yaml:"queue_size"`
}

// IsolationOverride for per-agent customization
type IsolationOverride struct {
	MemoryLimit string `yaml:"memory_limit,omitempty"`
	Timeout     string `yaml:"timeout,omitempty"`
}

// ParsedScalingConfig contains parsed duration fields
type ParsedScalingConfig struct {
	MinWorkers         int
	MaxWorkers         int
	TargetUtilization  float64
	ScaleUpThreshold   float64
	ScaleDownThreshold float64
	ScaleUpDelay       time.Duration
	ScaleDownDelay     time.Duration
	QueueSize          int
	IdleTimeout        time.Duration
}

// ParsedIsolationDefaults contains parsed size/duration fields
type ParsedIsolationDefaults struct {
	Enabled     bool
	Type        string
	MemoryLimit int           // MB
	Timeout     time.Duration // Parsed from string
}
