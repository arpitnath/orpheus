package daemon

import "orpheus/daemon/pkg/telemetry"

// LabelProvider provides custom telemetry labels for agents.
// PoolManager implements this interface.
type LabelProvider interface {
	GetLabelsForAgent(agentName string) []telemetry.Label
}

// MergeLabels combines base labels with custom agent labels.
// Returns base labels if custom is nil or empty.
func MergeLabels(base []telemetry.Label, custom []telemetry.Label) []telemetry.Label {
	if len(custom) == 0 {
		return base
	}
	merged := make([]telemetry.Label, len(base)+len(custom))
	copy(merged, base)
	copy(merged[len(base):], custom)
	return merged
}
