package scaling

import "time"

// TierScalingConfig holds the default scaling configuration for a pricing tier.
// Each tier (free, pro, teams) has different limits and scaling behavior.
type TierScalingConfig struct {
	// MinWorkers is the minimum number of workers to maintain.
	MinWorkers int

	// MaxWorkers is the maximum number of workers allowed.
	MaxWorkers int

	// InitialWorkers is the number of workers to spawn at startup.
	InitialWorkers int

	// TargetUtilization is the ideal ratio of tasks to workers.
	TargetUtilization float64

	// ScaleUpThreshold triggers scaling up when utilization exceeds this.
	ScaleUpThreshold float64

	// ScaleDownThreshold triggers scaling down when utilization falls below this.
	ScaleDownThreshold float64

	// ScaleUpDelay is the minimum time between scale-up operations.
	ScaleUpDelay time.Duration

	// ScaleDownDelay is the minimum time between scale-down operations.
	ScaleDownDelay time.Duration

	// IdleTimeout is how long a worker can be idle before termination.
	IdleTimeout time.Duration

	// QueueSize is the maximum number of pending requests allowed.
	QueueSize int
}

// DefaultTierConfigs maps tier names to their default configurations.
// These values represent the SLA differences between pricing tiers:
// - free: Limited resources, slower scaling (cost optimization)
// - pro: Balanced resources, moderate scaling (typical use)
// - teams: Maximum resources, aggressive scaling (enterprise needs)
var DefaultTierConfigs = map[string]TierScalingConfig{
	"free": {
		MinWorkers:         1,
		MaxWorkers:         2,
		InitialWorkers:     1,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   4.0,
		ScaleDownThreshold: 0.5,
		ScaleUpDelay:       30 * time.Second,
		ScaleDownDelay:     2 * time.Minute,
		IdleTimeout:        5 * time.Minute,
		QueueSize:          10,
	},
	"pro": {
		MinWorkers:         1,
		MaxWorkers:         10,
		InitialWorkers:     1,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   3.0,
		ScaleDownThreshold: 0.5,
		ScaleUpDelay:       15 * time.Second,
		ScaleDownDelay:     1 * time.Minute,
		IdleTimeout:        10 * time.Minute,
		QueueSize:          50,
	},
	"teams": {
		MinWorkers:         2,
		MaxWorkers:         50,
		InitialWorkers:     2,
		TargetUtilization:  2.0,
		ScaleUpThreshold:   2.5,
		ScaleDownThreshold: 0.5,
		ScaleUpDelay:       10 * time.Second,
		ScaleDownDelay:     30 * time.Second,
		IdleTimeout:        15 * time.Minute,
		QueueSize:          200,
	},
}

// GetTierPolicy returns a ScalingPolicy for the given tier name.
// If the tier is not found, it falls back to the "free" tier defaults.
func GetTierPolicy(tier string) ScalingPolicy {
	config, ok := DefaultTierConfigs[tier]
	if !ok {
		config = DefaultTierConfigs["free"]
	}
	return config.ToScalingPolicy()
}

// GetTierConfig returns the TierScalingConfig for the given tier name.
// If the tier is not found, it falls back to the "free" tier defaults.
func GetTierConfig(tier string) TierScalingConfig {
	config, ok := DefaultTierConfigs[tier]
	if !ok {
		return DefaultTierConfigs["free"]
	}
	return config
}

// ToScalingPolicy converts a TierScalingConfig to a ScalingPolicy.
// This is used to extract the scaling-specific fields for the autoscaler.
func (t TierScalingConfig) ToScalingPolicy() ScalingPolicy {
	return ScalingPolicy{
		MinWorkers:         t.MinWorkers,
		MaxWorkers:         t.MaxWorkers,
		TargetUtilization:  t.TargetUtilization,
		ScaleUpThreshold:   t.ScaleUpThreshold,
		ScaleDownThreshold: t.ScaleDownThreshold,
		ScaleUpDelay:       t.ScaleUpDelay,
		ScaleDownDelay:     t.ScaleDownDelay,
		IdleTimeout:        t.IdleTimeout,
	}
}

// ValidTiers returns the list of valid tier names.
func ValidTiers() []string {
	return []string{"free", "pro", "teams"}
}

// IsValidTier checks if the given tier name is valid.
func IsValidTier(tier string) bool {
	_, ok := DefaultTierConfigs[tier]
	return ok
}
