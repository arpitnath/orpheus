import React from 'react';
import { Text } from 'ink';

interface WorkerDotsProps {
  active: number;
  total: number;
}

export const WorkerDots: React.FC<WorkerDotsProps> = ({ active, total }) => {
  const activeDots = Math.min(active, total);
  const inactiveDots = total - activeDots;

  return (
    <Text>
      <Text color="green">{'●'.repeat(activeDots)}</Text>
      <Text dimColor>{'○'.repeat(inactiveDots)}</Text>
    </Text>
  );
};
