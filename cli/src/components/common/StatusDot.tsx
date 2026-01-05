import React from 'react';
import { Text } from 'ink';

export type StatusType =
  | 'healthy'
  | 'degraded'
  | 'unhealthy'
  | 'running'
  | 'idle'
  | 'error'
  | 'pending'
  | 'stopped';

interface StatusDotProps {
  status: StatusType | null | undefined;
  showLabel?: boolean;
}

interface StatusConfig {
  icon: string;
  color: string;
  label: string;
}

function getStatusConfig(status: StatusType | null | undefined): StatusConfig {
  switch (status) {
    case 'healthy':
    case 'running':
      return { icon: '●', color: 'green', label: status };
    case 'degraded':
    case 'idle':
      return { icon: '!', color: 'yellow', label: status };
    case 'unhealthy':
    case 'error':
    case 'stopped':
      return { icon: '✗', color: 'red', label: status };
    case 'pending':
    default:
      return { icon: '○', color: 'gray', label: status || 'unknown' };
  }
}

export const StatusDot: React.FC<StatusDotProps> = ({ status, showLabel = false }) => {
  const config = getStatusConfig(status);

  return (
    <Text color={config.color}>
      {config.icon}
      {showLabel && ` ${config.label}`}
    </Text>
  );
};
