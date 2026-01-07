import React from 'react';
import { Box, Text } from 'ink';

interface SectionProps {
  title: string;
  children: React.ReactNode;
  marginTop?: number;
}

export const Section: React.FC<SectionProps> = ({
  title,
  children,
  marginTop = 1,
}) => {
  return (
    <Box flexDirection="column" marginTop={marginTop}>
      <Text color="cyan">{title}</Text>
      <Text dimColor>{'─'.repeat(title.length)}</Text>
      <Box flexDirection="column">{children}</Box>
    </Box>
  );
};
