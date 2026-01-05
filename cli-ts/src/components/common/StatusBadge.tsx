import React from 'react';
import { Text } from 'ink';

export type BadgeStatus = 'deployed' | 'running' | 'idle' | 'stopped' | 'error' | 'pending' | 'connected' | 'disconnected';

interface StatusBadgeProps {
  status: BadgeStatus;
}

interface BadgeConfig {
  label: string;
  backgroundColor: string;
  color: string;
}

function getBadgeConfig(status: BadgeStatus): BadgeConfig {
  switch (status) {
    case 'deployed':
      return { label: 'DEPLOYED', backgroundColor: 'blue', color: 'white' };
    case 'running':
      return { label: 'RUNNING', backgroundColor: 'green', color: 'white' };
    case 'idle':
      return { label: 'IDLE', backgroundColor: 'gray', color: 'white' };
    case 'stopped':
      return { label: 'STOPPED', backgroundColor: 'yellow', color: 'black' };
    case 'error':
      return { label: 'ERROR', backgroundColor: 'red', color: 'white' };
    case 'connected':
      return { label: 'CONNECTED', backgroundColor: 'green', color: 'white' };
    case 'disconnected':
      return { label: 'DISCONNECTED', backgroundColor: 'red', color: 'white' };
    case 'pending':
    default:
      return { label: 'PENDING', backgroundColor: 'gray', color: 'white' };
  }
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status }) => {
  const config = getBadgeConfig(status);

  return (
    <Text backgroundColor={config.backgroundColor} color={config.color}>
      {` ${config.label} `}
    </Text>
  );
};
