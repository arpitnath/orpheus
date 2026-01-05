//@STATUS_COMPONENT
import React, { useCallback } from 'react';
import { Box, Text, useApp } from 'ink';
import { useHealth, useStats, useRefreshAndQuit } from '../hooks/index.js';
import { Spinner, StatusDot, RefreshHint } from './common/index.js';
import { getActiveServerName, getActiveServer } from '../lib/config.js';
import { formatUptime } from '../lib/format.js';
import type { HealthResponse, StatsResponse } from '../types/index.js';

//@HELPERS
function getStatusType(
  health: HealthResponse | null,
  stats: StatsResponse | null
): 'healthy' | 'degraded' | 'unhealthy' | 'error' {
  if (!health) {
    return 'error';
  }

  // Check for degraded state: workers=0 but queue>0
  if (stats?.global) {
    const { total_workers, total_pending } = stats.global;
    if (total_workers === 0 && total_pending > 0) {
      return 'degraded';
    }
  }

  return health.status;
}

function getHint(health: HealthResponse | null, stats: StatsResponse | null): string | null {
  if (!health) {
    return 'Run: orpheus vm start';
  }

  if (stats?.global) {
    const { total_workers, total_pending } = stats.global;
    if (total_workers === 0 && total_pending > 0) {
      return `${total_pending} requests queued, no workers available`;
    }
  }

  return null;
}

function getServerUrl(): string {
  const server = getActiveServer();
  if (server.mode === 'tcp' && server.url) {
    return server.url;
  }
  if (server.mode === 'unix_socket' && server.socket_path) {
    return server.socket_path;
  }
  return '';
}

//@COMPONENT
export const Status: React.FC = () => {
  const { exit } = useApp();

  // Use hooks for data fetching
  const { health, loading: healthLoading, refetch: refetchHealth } = useHealth();
  const { stats, refetch: refetchStats } = useStats();

  // Combined refetch for refresh key
  const refetch = useCallback(() => {
    refetchHealth();
    refetchStats();
  }, [refetchHealth, refetchStats]);

  // Handle keyboard input (r to refresh, q to quit)
  useRefreshAndQuit(refetch, exit);

  // Get server info
  const serverName = getActiveServerName();
  const serverUrl = getServerUrl();

  // Loading state
  if (healthLoading) {
    return (
      <Box>
        <Spinner label="Checking status..." />
      </Box>
    );
  }

  // Derive display values
  const statusType = getStatusType(health, stats);
  const hint = getHint(health, stats);
  const uptime = health ? formatUptime(health.uptime_seconds) : '';
  const agents = stats?.global?.total_agents ?? 0;
  const workers = stats?.global?.total_workers ?? 0;
  const queue = stats?.global?.total_pending ?? 0;

  return (
    <Box flexDirection="column" paddingY={1}>
      {/* Row 1: Status indicator + Server name */}
      <Box justifyContent="space-between" width={60}>
        <StatusDot status={statusType} showLabel />
        <Text>{serverName}</Text>
      </Box>

      {/* Row 2: Uptime + URL */}
      <Box justifyContent="space-between" width={60}>
        {health ? (
          <Text dimColor>  Uptime {uptime}</Text>
        ) : (
          <Text> </Text>
        )}
        <Text dimColor>{serverUrl}</Text>
      </Box>

      {/* Stats row (only if daemon is running) */}
      {health && (
        <>
          <Box marginY={1} />
          <Box gap={4}>
            <Box>
              <Text dimColor>  Agents</Text>
              <Text>     {agents}</Text>
            </Box>
            <Box>
              <Text dimColor>Workers</Text>
              <Text>    {workers}</Text>
            </Box>
            <Box>
              <Text dimColor>Queue</Text>
              <Text>    {queue}</Text>
            </Box>
          </Box>
        </>
      )}

      {/* Hint (only if there's an issue) */}
      {hint && (
        <Box marginTop={1}>
          <Text color="cyan">  → {hint}</Text>
        </Box>
      )}

      {/* Refresh hint */}
      <RefreshHint />
    </Box>
  );
};

export default Status;
