import React from 'react';
import { Box, Text } from 'ink';

interface RowProps {
  label: string;
  value: React.ReactNode;
  labelWidth?: number;
  dimLabel?: boolean;
}

export const Row: React.FC<RowProps> = ({
  label,
  value,
  labelWidth = 16,
  dimLabel = true,
}) => {
  return (
    <Box>
      <Text dimColor={dimLabel}>{label.padEnd(labelWidth)}</Text>
      {typeof value === 'string' ? <Text>{value}</Text> : value}
    </Box>
  );
};
