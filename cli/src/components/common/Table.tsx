import React from 'react';
import { Box, Text } from 'ink';

export interface Column {
  key: string;
  header: string;
  width: number;
  color?: string;
  align?: 'left' | 'right';
}

interface TableProps<T> {
  columns: Column[];
  data: T[];
  renderCell: (item: T, column: Column) => React.ReactNode;
  emptyMessage?: string;
}

export function Table<T>({
  columns,
  data,
  renderCell,
  emptyMessage = 'No data',
}: TableProps<T>): React.ReactElement {
  if (data.length === 0) {
    return <Text dimColor>{emptyMessage}</Text>;
  }

  const totalWidth = columns.reduce((sum, col) => sum + col.width, 0);

  return (
    <Box flexDirection="column">
      {/* Header */}
      <Box>
        {columns.map((col) => (
          <Text key={col.key} dimColor>
            {col.align === 'right'
              ? col.header.padStart(col.width)
              : col.header.padEnd(col.width)}
          </Text>
        ))}
      </Box>

      {/* Separator */}
      <Text dimColor>{'─'.repeat(totalWidth)}</Text>

      {/* Rows */}
      {data.map((item, index) => (
        <Box key={index}>
          {columns.map((col) => {
            const content = renderCell(item, col);
            if (typeof content === 'string') {
              return (
                <Text key={col.key} color={col.color}>
                  {col.align === 'right'
                    ? content.padStart(col.width)
                    : content.padEnd(col.width)}
                </Text>
              );
            }
            return (
              <Box key={col.key} width={col.width}>
                {content}
              </Box>
            );
          })}
        </Box>
      ))}
    </Box>
  );
}
