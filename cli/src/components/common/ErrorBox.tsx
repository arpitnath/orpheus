import React from 'react';
import { Box, Text } from 'ink';

interface ErrorBoxProps {
  message: string;
  hint?: string;
}

export const ErrorBox: React.FC<ErrorBoxProps> = ({ message, hint }) => {
  return (
    <Box flexDirection="column" paddingY={1}>
      <Text color="red">✗ {message}</Text>
      {hint && (
        <Box marginTop={1}>
          <Text color="cyan">  → {hint}</Text>
        </Box>
      )}
    </Box>
  );
};
