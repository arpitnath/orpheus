import React from 'react';
import { Box, Text, useStdin } from 'ink';

interface RefreshHintProps {
  showQuit?: boolean;
}

export const RefreshHint: React.FC<RefreshHintProps> = ({ showQuit = true }) => {
  const { isRawModeSupported } = useStdin();

  // Only show hint when in interactive mode
  if (!isRawModeSupported) {
    return null;
  }

  return (
    <Box marginTop={1}>
      <Text dimColor>
        Press r to refresh{showQuit ? ', q to quit' : ''}
      </Text>
    </Box>
  );
};
