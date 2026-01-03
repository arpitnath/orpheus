//@BADGE_COMPONENT
import React from 'react';
import { Text } from 'ink';

type BadgeStatus = 'healthy' | 'degraded' | 'unhealthy' | 'running' | 'idle' | 'stopped' | 'unknown';

interface BadgeProps {
  status: BadgeStatus;
  label?: string;
}

const statusConfig: Record<BadgeStatus, { color: string; icon: string }> = {
  healthy: { color: 'green', icon: '●' },
  running: { color: 'green', icon: '●' },
  degraded: { color: 'yellow', icon: '●' },
  idle: { color: 'yellow', icon: '○' },
  unhealthy: { color: 'red', icon: '●' },
  stopped: { color: 'red', icon: '●' },
  unknown: { color: 'gray', icon: '○' },
};

export const Badge: React.FC<BadgeProps> = ({ status, label }) => {
  const config = statusConfig[status] || statusConfig.unknown;
  const displayLabel = label || status;

  return (
    <Text>
      <Text color={config.color}>{config.icon}</Text>
      <Text color={config.color}> {displayLabel}</Text>
    </Text>
  );
};
