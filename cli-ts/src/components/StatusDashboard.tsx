//@STATUS_DASHBOARD
import React, { useState, useEffect } from 'react';
import { Box, Text, useApp, useInput } from 'ink';
import { Badge } from './common/Badge.js';
import { Spinner } from './common/Spinner.js';
import { getHealth, getStats } from '../lib/api.js';
import { getActiveServerName, socketExists, getDefaultSocketPath } from '../lib/config.js';
import type { HealthResponse, StatsResponse } from '../types/index.js';

//@HELPERS
function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${minutes}m`;
}

//@COMPONENT
export const StatusDashboard: React.FC = () => {
  const { exit } = useApp();
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useInput((input, key) => {
    if (input === 'q' || key.escape) {
      exit();
    }
  });

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [healthData, statsData] = await Promise.all([
          getHealth(),
          getStats(),
        ]);
        setHealth(healthData);
        setStats(statsData);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      } finally {
        setLoading(false);
      }
    };

    fetchData();
    const interval = setInterval(fetchData, 2000);
    return () => clearInterval(interval);
  }, []);

  const serverName = getActiveServerName();
  const hasSocket = socketExists();

  if (!hasSocket) {
    return (
      <Box flexDirection="column" padding={1}>
        <Box borderStyle="round" borderColor="cyan" paddingX={2} paddingY={1}>
          <Text bold color="cyan">Orpheus Status</Text>
        </Box>
        <Box marginTop={1} flexDirection="column">
          <Text>Server: {serverName}</Text>
          <Text>Daemon: <Text color="red">not running</Text> (socket not found)</Text>
          <Text dimColor>Socket: {getDefaultSocketPath()}</Text>
        </Box>
        <Box marginTop={1}>
          <Text dimColor>Press 'q' to quit</Text>
        </Box>
      </Box>
    );
  }

  if (loading) {
    return (
      <Box flexDirection="column" padding={1}>
        <Box borderStyle="round" borderColor="cyan" paddingX={2} paddingY={1}>
          <Text bold color="cyan">Orpheus Status</Text>
        </Box>
        <Box marginTop={1}>
          <Spinner label="Connecting to daemon..." />
        </Box>
      </Box>
    );
  }

  if (error || !health) {
    return (
      <Box flexDirection="column" padding={1}>
        <Box borderStyle="round" borderColor="cyan" paddingX={2} paddingY={1}>
          <Text bold color="cyan">Orpheus Status</Text>
        </Box>
        <Box marginTop={1} flexDirection="column">
          <Text>Server: {serverName}</Text>
          <Text>Daemon: <Text color="red">not responding</Text></Text>
          {error && <Text dimColor>Error: {error}</Text>}
        </Box>
        <Box marginTop={1}>
          <Text dimColor>Press 'q' to quit</Text>
        </Box>
      </Box>
    );
  }

  const daemonStatus = health.status as 'healthy' | 'degraded' | 'unhealthy';

  return (
    <Box flexDirection="column" padding={1}>
      <Box borderStyle="round" borderColor="cyan" paddingX={2} paddingY={1}>
        <Text bold color="cyan">Orpheus Status</Text>
      </Box>

      <Box marginTop={1} flexDirection="column" gap={0}>
        <Box>
          <Text>Daemon: </Text>
          <Badge status={daemonStatus} />
          <Text>     Uptime: {formatUptime(health.uptime_seconds)}</Text>
        </Box>
        <Text>Server: {serverName}</Text>
      </Box>

      {stats && stats.global && (
        <Box marginTop={1} flexDirection="column" gap={0}>
          <Text>Agents: {stats.global.total_agents} deployed</Text>
          <Text>
            Workers: {stats.global.total_workers} total
            {stats.global.total_workers > 0 && (
              <Text dimColor>
                {' '}({stats.global.total_workers - stats.global.total_pending} busy, {stats.global.total_pending} idle)
              </Text>
            )}
          </Text>
          <Text>Pending: {stats.global.total_pending} requests</Text>
        </Box>
      )}

      <Box marginTop={1}>
        <Text dimColor>Press 'q' to quit</Text>
      </Box>
    </Box>
  );
};
