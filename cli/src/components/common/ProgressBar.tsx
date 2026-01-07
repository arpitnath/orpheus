import React from 'react';
import { Text } from 'ink';

interface ProgressBarProps {
  value: number;
  max: number;
  width?: number;
  showPercent?: boolean;
  colorByValue?: boolean;
}

function getColor(percent: number, colorByValue: boolean): string {
  if (!colorByValue) return 'green';
  if (percent < 60) return 'green';
  if (percent < 80) return 'yellow';
  return 'red';
}

export const ProgressBar: React.FC<ProgressBarProps> = ({
  value,
  max,
  width = 10,
  showPercent = false,
  colorByValue = false,
}) => {
  const percent = max > 0 ? Math.round((value / max) * 100) : 0;
  const filled = Math.round((percent / 100) * width);
  const empty = width - filled;
  const color = getColor(percent, colorByValue);

  return (
    <Text>
      <Text color={color}>{'█'.repeat(filled)}</Text>
      <Text dimColor>{'░'.repeat(empty)}</Text>
      {showPercent && <Text> {percent}%</Text>}
    </Text>
  );
};
