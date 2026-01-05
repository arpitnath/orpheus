import React from 'react';
import { Box, Text } from 'ink';
import { Spinner } from './Spinner.js';

export type CheckStatus = 'pending' | 'running' | 'passed' | 'failed' | 'warning' | 'skipped';

interface CheckItemProps {
  status: CheckStatus;
  label: string;
  timing?: string;
  error?: string;
  fix?: string;
  labelWidth?: number;
}

interface StatusConfig {
  icon: string;
  color: string;
}

function getStatusConfig(status: CheckStatus): StatusConfig {
  switch (status) {
    case 'passed':
      return { icon: '✓', color: 'green' };
    case 'failed':
      return { icon: '✗', color: 'red' };
    case 'warning':
      return { icon: '!', color: 'yellow' };
    case 'skipped':
      return { icon: '○', color: 'gray' };
    case 'running':
      return { icon: '◐', color: 'cyan' };
    case 'pending':
    default:
      return { icon: '○', color: 'gray' };
  }
}

export const CheckItem: React.FC<CheckItemProps> = ({
  status,
  label,
  timing,
  error,
  fix,
  labelWidth = 40,
}) => {
  const config = getStatusConfig(status);

  return (
    <Box flexDirection="column">
      <Box>
        {status === 'running' ? (
          <Spinner />
        ) : (
          <Text color={config.color}>{config.icon}</Text>
        )}
        <Text> {label.padEnd(labelWidth)}</Text>
        {timing && <Text dimColor>{timing}</Text>}
      </Box>

      {status === 'failed' && error && (
        <Box marginLeft={4} flexDirection="column">
          <Text dimColor>{error}</Text>
          {fix && (
            <Box marginTop={0}>
              <Text color="cyan">Fix: </Text>
              <Text>{fix}</Text>
            </Box>
          )}
        </Box>
      )}

      {status === 'warning' && error && (
        <Box marginLeft={4}>
          <Text color="yellow">{error}</Text>
        </Box>
      )}
    </Box>
  );
};
