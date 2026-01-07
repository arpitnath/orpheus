//@LOG_VIEWER
import React, { useState, useEffect } from 'react';
import { Box, Text, useApp, useInput } from 'ink';

type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';

interface LogEntry {
  timestamp: string;
  level: LogLevel;
  message: string;
  agent?: string;
}

interface LogViewerProps {
  follow?: boolean;
  tail?: number;
  grep?: string;
  onFetchLogs: () => Promise<LogEntry[]>;
}

//@LEVEL_COLOR
const levelColors: Record<LogLevel, string> = {
  DEBUG: 'gray',
  INFO: 'cyan',
  WARN: 'yellow',
  ERROR: 'red',
};

//@COMPONENT
export const LogViewer: React.FC<LogViewerProps> = ({
  follow = false,
  tail = 50,
  grep,
  onFetchLogs,
}) => {
  const { exit } = useApp();
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [paused, setPaused] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useInput((input, key) => {
    if (input === 'q' || key.escape) {
      exit();
    }
    if (input === ' ') {
      setPaused(p => !p);
    }
  });

  useEffect(() => {
    const fetchLogs = async () => {
      try {
        const newLogs = await onFetchLogs();
        let filtered = newLogs;

        // Apply grep filter
        if (grep) {
          const pattern = new RegExp(grep, 'i');
          filtered = filtered.filter(log => pattern.test(log.message));
        }

        // Apply tail limit
        if (tail && filtered.length > tail) {
          filtered = filtered.slice(-tail);
        }

        setLogs(filtered);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch logs');
      }
    };

    fetchLogs();

    if (follow && !paused) {
      const interval = setInterval(fetchLogs, 1000);
      return () => clearInterval(interval);
    }
    return undefined;
  }, [follow, paused, tail, grep, onFetchLogs]);

  const statusText = follow
    ? paused
      ? 'paused'
      : 'following'
    : 'static';

  return (
    <Box flexDirection="column" padding={1}>
      <Box marginBottom={1}>
        <Text bold>Logs</Text>
        <Text dimColor> ({statusText})</Text>
        <Text dimColor>  Press 'q' to quit</Text>
        {follow && <Text dimColor>, space to {paused ? 'resume' : 'pause'}</Text>}
      </Box>

      {error ? (
        <Text color="red">Error: {error}</Text>
      ) : logs.length === 0 ? (
        <Text dimColor>No logs available</Text>
      ) : (
        <Box flexDirection="column">
          {logs.map((log, i) => (
            <Box key={i}>
              <Text dimColor>[{log.timestamp}]</Text>
              <Text> </Text>
              <Text color={levelColors[log.level]}>{log.level.padEnd(5)}</Text>
              <Text> </Text>
              {log.agent && <Text color="magenta">[{log.agent}] </Text>}
              <Text>{log.message}</Text>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
};
